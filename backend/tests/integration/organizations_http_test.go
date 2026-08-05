package integration_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/app"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	organizationsapplication "file-workshop/backend/internal/modules/organizations/application"
	organizationsdomain "file-workshop/backend/internal/modules/organizations/domain"
	organizationsrepository "file-workshop/backend/internal/modules/organizations/repository"
	"file-workshop/backend/internal/platform/config"
	"file-workshop/backend/internal/platform/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestOrganizationsHTTPAndQuotaLifecycle(t *testing.T) {
	if os.Getenv(integrationEnvironment) != "1" {
		t.Skip("set FILE_WORKSHOP_RUN_INTEGRATION=1 to run local dependency integration tests")
	}
	backendRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve backend root: %v", err)
	}
	t.Setenv("FILE_WORKSHOP_ENV_FILE", filepath.Join(backendRoot, ".env"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	connection, err := pgx.Connect(ctx, cfg.PostgreSQL.ConnectionString())
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	if _, err := connection.Exec(ctx, "SET search_path TO "+pgx.Identifier{cfg.PostgreSQL.Schema}.Sanitize()+",public"); err != nil {
		t.Fatalf("set search path: %v", err)
	}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	adminID, userID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	adminCredentialID, userCredentialID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	adminUsername, userUsername := "organizations_admin_"+suffix, "organizations_user_"+suffix
	adminPassword, userPassword := "Admin-"+suffix+"!Aa1", "User-"+suffix+"!Bb2"
	organizationIDs := make([]uuid.UUID, 0, 8)
	spaceIDs := make([]uuid.UUID, 0, 8)

	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		stopIDs := []uuid.UUID{adminID, userID}
		_, _ = connection.Exec(cleanupContext, "DELETE FROM quota_reservations WHERE user_id = ANY($1::uuid[]) OR space_id = ANY($2::uuid[])", stopIDs, spaceIDs)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM organization_change_operations WHERE organization_change_plan_id IN (SELECT organization_change_plan_id FROM organization_change_plans WHERE created_by_user_id = $1)", adminID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM organization_change_plans WHERE created_by_user_id = $1", adminID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM outbox_events WHERE aggregate_id = ANY($1::uuid[])", organizationIDs)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM idempotency_records WHERE user_id = $1", adminID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM user_organizations WHERE user_id = ANY($1::uuid[]) OR organization_id = ANY($2::uuid[])", stopIDs, organizationIDs)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM spaces WHERE owner_user_id = ANY($1::uuid[]) OR organization_id = ANY($2::uuid[])", stopIDs, organizationIDs)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM organization_security_versions WHERE organization_id = ANY($1::uuid[])", organizationIDs)
		_, _ = connection.Exec(cleanupContext, "UPDATE organizations SET parent_organization_id = NULL WHERE organization_id = ANY($1::uuid[])", organizationIDs)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM organization_closure WHERE ancestor_organization_id = ANY($1::uuid[]) OR descendant_organization_id = ANY($1::uuid[])", organizationIDs)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM organizations WHERE organization_id = ANY($1::uuid[])", organizationIDs)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM login_attempts WHERE username_normalized = ANY($1::text[])", []string{adminUsername, userUsername})
		_, _ = connection.Exec(cleanupContext, "DELETE FROM session_refresh_tokens WHERE user_session_id IN (SELECT user_session_id FROM user_sessions WHERE user_id = ANY($1::uuid[]))", stopIDs)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM user_sessions WHERE user_id = ANY($1::uuid[])", stopIDs)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM user_credentials WHERE user_id = ANY($1::uuid[])", stopIDs)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM principal_security_versions WHERE user_id = ANY($1::uuid[])", stopIDs)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM users WHERE user_id = ANY($1::uuid[])", stopIDs)
		_ = connection.Close(cleanupContext)
	})

	hasher := identitydomain.NewArgon2IDHasher()
	adminHash, err := hasher.Hash(adminPassword)
	if err != nil {
		t.Fatalf("hash administrator password: %v", err)
	}
	userHash, err := hasher.Hash(userPassword)
	if err != nil {
		t.Fatalf("hash user password: %v", err)
	}
	now := time.Now().UTC()
	for _, fixture := range []struct {
		id, credentialID                  uuid.UUID
		username, displayName, role, hash string
	}{
		{adminID, adminCredentialID, adminUsername, "Organizations Admin", "SYSTEM_ADMIN", adminHash},
		{userID, userCredentialID, userUsername, "Organizations User", "USER", userHash},
	} {
		if _, err := connection.Exec(ctx, `INSERT INTO users (user_id, username, username_normalized, display_name, system_role, status, locale, timezone, created_at, updated_at) VALUES ($1,$2,$2,$3,$4,'ACTIVE','zh-CN','Asia/Shanghai',$5,$5)`, fixture.id, fixture.username, fixture.displayName, fixture.role, now); err != nil {
			t.Fatalf("insert user fixture: %v", err)
		}
		if _, err := connection.Exec(ctx, `INSERT INTO user_credentials (user_credential_id,user_id,credential_type,identifier,identifier_normalized,secret_hash,status,created_at,updated_at) VALUES ($1,$2,'PASSWORD',$3,$3,$4,'ACTIVE',$5,$5)`, fixture.credentialID, fixture.id, fixture.username, fixture.hash, now); err != nil {
			t.Fatalf("insert credential fixture: %v", err)
		}
		if _, err := connection.Exec(ctx, "INSERT INTO principal_security_versions (user_id, updated_at) VALUES ($1,$2)", fixture.id, now); err != nil {
			t.Fatalf("insert principal versions: %v", err)
		}
	}

	cfg.HTTP.Port = availablePort(t)
	cfg.HTTP.ShutdownTimeout = 5 * time.Second
	serverContext, stopServer := context.WithCancel(context.Background())
	t.Cleanup(stopServer)
	serverErrors := make(chan error, 1)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	go func() { serverErrors <- app.RunServer(serverContext, cfg, logger) }()
	waitForReadiness(t, cfg.HTTP.Address(), serverErrors)
	baseURL := "http://" + cfg.HTTP.Address()
	client := &http.Client{Timeout: 10 * time.Second}
	adminLogin := postLogin(t, client, baseURL, adminUsername, adminPassword)
	userLogin := postLogin(t, client, baseURL, userUsername, userPassword)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminLogin.AccessToken, "Content-Type": "application/json"}
	userHeaders := map[string]string{"Authorization": "Bearer " + userLogin.AccessToken, "Content-Type": "application/json"}

	createOrganization := func(name string, parentID *uuid.UUID, quota int64) api.Organization {
		t.Helper()
		body := map[string]any{"name": name, "code": strings.ToUpper(name) + "-" + suffix[:6], "spaceQuotaBytes": quota}
		if parentID != nil {
			body["parentOrganizationId"] = parentID.String()
		}
		headers := cloneHeaders(adminHeaders)
		headers["Idempotency-Key"] = "org-" + strings.ToLower(name) + "-" + suffix
		response := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/organizations", bytes.NewReader(mustJSON(t, body)), headers)
		assertStatus(t, response, http.StatusCreated)
		var result api.OrganizationResponse
		decodeResponse(t, response, &result)
		organizationIDs = append(organizationIDs, uuid.UUID(result.Organization.OrganizationId))
		return result.Organization
	}

	root := createOrganization("Root", nil, 4096)
	replayHeaders := cloneHeaders(adminHeaders)
	replayHeaders["Idempotency-Key"] = "org-root-" + suffix
	replayPayload := mustJSON(t, map[string]any{"name": "Root", "code": "ROOT-" + suffix[:6], "spaceQuotaBytes": int64(4096)})
	replayResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/organizations", bytes.NewReader(replayPayload), replayHeaders)
	assertStatus(t, replayResponse, http.StatusCreated)
	var replayed api.OrganizationResponse
	decodeResponse(t, replayResponse, &replayed)
	if replayed.Organization.OrganizationId != root.OrganizationId {
		t.Fatalf("organization idempotent replay returned a different resource")
	}
	child := createOrganization("Child", ptrUUID(uuid.UUID(root.OrganizationId)), 2048)
	targetA := createOrganization("TargetA", nil, 2048)
	targetB := createOrganization("TargetB", nil, 2048)

	listResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/organizations?page=1&pageSize=2&status=ACTIVE", nil, adminHeaders)
	assertStatus(t, listResponse, http.StatusOK)
	var organizations api.OrganizationListResponse
	decodeResponse(t, listResponse, &organizations)
	if organizations.Page != 1 || organizations.PageSize != 2 || len(organizations.Items) != 2 || organizations.Total < 4 {
		t.Fatalf("unexpected organization pagination: %#v", organizations)
	}
	forbiddenResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/organizations", nil, userHeaders)
	assertErrorCode(t, forbiddenResponse, http.StatusForbidden, "AUTH_FORBIDDEN")

	memberHeaders := cloneHeaders(adminHeaders)
	memberHeaders["Idempotency-Key"] = "member-primary-" + suffix
	memberPayload := mustJSON(t, map[string]any{"userId": userID.String(), "membershipType": "PRIMARY", "jobTitle": "Operator"})
	memberResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/organizations/"+uuid.UUID(root.OrganizationId).String()+"/members", bytes.NewReader(memberPayload), memberHeaders)
	assertStatus(t, memberResponse, http.StatusCreated)
	var membership api.OrganizationMembershipResponse
	decodeResponse(t, memberResponse, &membership)
	duplicateMemberHeaders := cloneHeaders(adminHeaders)
	duplicateMemberHeaders["Idempotency-Key"] = "member-duplicate-" + suffix
	duplicateMemberResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/organizations/"+uuid.UUID(targetA.OrganizationId).String()+"/members", bytes.NewReader(memberPayload), duplicateMemberHeaders)
	assertErrorCode(t, duplicateMemberResponse, http.StatusConflict, "ORGANIZATION_CONFLICT")
	futureHeaders := cloneHeaders(adminHeaders)
	futureHeaders["Idempotency-Key"] = "member-future-" + suffix
	futureResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/organizations/"+uuid.UUID(targetA.OrganizationId).String()+"/members", bytes.NewReader(mustJSON(t, map[string]any{"userId": userID.String(), "membershipType": "MEMBER", "effectiveFrom": now.Add(24 * time.Hour)})), futureHeaders)
	assertStatus(t, futureResponse, http.StatusCreated)
	_ = futureResponse.Body.Close()
	currentMembershipsResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/users/me/organizations?page=1&pageSize=50", nil, userHeaders)
	assertStatus(t, currentMembershipsResponse, http.StatusOK)
	var currentMemberships api.OrganizationMembershipListResponse
	decodeResponse(t, currentMembershipsResponse, &currentMemberships)
	if len(currentMemberships.Items) != 1 || currentMemberships.Items[0].MembershipId != membership.Membership.MembershipId {
		t.Fatalf("current memberships did not apply effective period: %#v", currentMemberships.Items)
	}
	updateMemberResponse := doRequest(t, client, http.MethodPatch, baseURL+"/api/v1/admin/organizations/"+uuid.UUID(root.OrganizationId).String()+"/members/"+uuid.UUID(membership.Membership.MembershipId).String(), bytes.NewReader(mustJSON(t, map[string]any{"jobTitle": "Senior Operator", "rowVersion": membership.Membership.RowVersion})), adminHeaders)
	assertStatus(t, updateMemberResponse, http.StatusOK)
	decodeResponse(t, updateMemberResponse, &membership)
	if membership.Membership.JobTitle == nil || *membership.Membership.JobTitle != "Senior Operator" {
		t.Fatalf("membership job title was not updated: %#v", membership.Membership)
	}

	personalHeaders := cloneHeaders(adminHeaders)
	personalHeaders["Idempotency-Key"] = "personal-" + suffix
	personalResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/users/"+userID.String()+"/personal-space", bytes.NewReader(mustJSON(t, map[string]any{"name": "Personal", "quotaBytes": int64(1000), "config": map[string]any{"theme": "industrial"}})), personalHeaders)
	assertStatus(t, personalResponse, http.StatusCreated)
	var personal api.SpaceResponse
	decodeResponse(t, personalResponse, &personal)
	spaceIDs = append(spaceIDs, uuid.UUID(personal.Space.SpaceId))
	currentSpaceResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/users/me/personal-space", nil, userHeaders)
	assertStatus(t, currentSpaceResponse, http.StatusOK)
	var currentSpace api.SpaceResponse
	decodeResponse(t, currentSpaceResponse, &currentSpace)
	if currentSpace.Space.SpaceId != personal.Space.SpaceId || currentSpace.Space.SpaceType != api.SpaceType("PERSONAL") {
		t.Fatalf("unexpected personal space: %#v", currentSpace.Space)
	}
	publicHeaders := cloneHeaders(adminHeaders)
	publicHeaders["Idempotency-Key"] = "public-" + suffix
	publicResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/spaces", bytes.NewReader(mustJSON(t, map[string]any{"name": "Public", "quotaBytes": int64(2000), "config": map[string]any{"classification": "internal"}})), publicHeaders)
	assertStatus(t, publicResponse, http.StatusCreated)
	var publicSpace api.SpaceResponse
	decodeResponse(t, publicResponse, &publicSpace)
	spaceIDs = append(spaceIDs, uuid.UUID(publicSpace.Space.SpaceId))
	updateSpaceResponse := doRequest(t, client, http.MethodPatch, baseURL+"/api/v1/admin/spaces/"+uuid.UUID(publicSpace.Space.SpaceId).String(), bytes.NewReader(mustJSON(t, map[string]any{"name": "Public Documents", "quotaBytes": int64(2500), "configSchemaVersion": 2, "config": map[string]any{"classification": "internal"}, "rowVersion": publicSpace.Space.RowVersion})), adminHeaders)
	assertStatus(t, updateSpaceResponse, http.StatusOK)
	decodeResponse(t, updateSpaceResponse, &publicSpace)
	freezeSpaceResponse := doRequest(t, client, http.MethodPut, baseURL+"/api/v1/admin/spaces/"+uuid.UUID(publicSpace.Space.SpaceId).String()+"/status", bytes.NewReader(mustJSON(t, map[string]any{"status": "FROZEN", "rowVersion": publicSpace.Space.RowVersion, "reason": "integration freeze"})), adminHeaders)
	assertStatus(t, freezeSpaceResponse, http.StatusOK)
	decodeResponse(t, freezeSpaceResponse, &publicSpace)
	if publicSpace.Space.Status != api.SpaceStatus("FROZEN") {
		t.Fatalf("public space status = %s, want FROZEN", publicSpace.Space.Status)
	}
	spacesResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/spaces?page=1&pageSize=50&spaceType=PUBLIC&status=FROZEN", nil, adminHeaders)
	assertStatus(t, spacesResponse, http.StatusOK)
	var spaces api.SpaceListResponse
	decodeResponse(t, spacesResponse, &spaces)
	if spaces.Total < 1 || len(spaces.Items) < 1 {
		t.Fatalf("public space list is empty: %#v", spaces)
	}

	pool, err := database.OpenPostgreSQL(ctx, cfg.App, cfg.PostgreSQL)
	if err != nil {
		t.Fatalf("open quota test pool: %v", err)
	}
	repository := organizationsrepository.NewPostgreSQL(pool)
	quotaService := organizationsapplication.NewService(repository, repository, time.Now)
	quotaResults := make(chan error, 2)
	startQuota := make(chan struct{})
	for range 2 {
		go func() {
			<-startQuota
			_, reserveErr := quotaService.ReserveQuota(context.Background(), organizationsapplication.ReserveQuotaInput{SpaceID: uuid.UUID(personal.Space.SpaceId), UserID: userID, ReservedBytes: 600, ExpiresAt: time.Now().Add(time.Hour)})
			quotaResults <- reserveErr
		}()
	}
	close(startQuota)
	quotaSuccess, quotaExceeded := 0, 0
	for range 2 {
		reserveErr := <-quotaResults
		switch {
		case reserveErr == nil:
			quotaSuccess++
		case strings.Contains(reserveErr.Error(), organizationsdomain.ErrQuotaExceeded.Error()):
			quotaExceeded++
		default:
			t.Fatalf("unexpected concurrent quota error: %v", reserveErr)
		}
	}
	if quotaSuccess != 1 || quotaExceeded != 1 {
		t.Fatalf("concurrent quota results success=%d exceeded=%d", quotaSuccess, quotaExceeded)
	}

	moveStatuses := make(chan int, 2)
	startMove := make(chan struct{})
	for _, target := range []api.Organization{targetA, targetB} {
		targetID := uuid.UUID(target.OrganizationId)
		go func() {
			<-startMove
			response := doRequestNoFatal(client, http.MethodPost, baseURL+"/api/v1/admin/organizations/"+uuid.UUID(child.OrganizationId).String()+"/move", mustJSON(t, map[string]any{"newParentOrganizationId": targetID.String(), "rowVersion": child.RowVersion, "reason": "concurrent move"}), adminHeaders)
			moveStatuses <- response
		}()
	}
	close(startMove)
	statuses := []int{<-moveStatuses, <-moveStatuses}
	sort.Ints(statuses)
	if statuses[0] != http.StatusOK || statuses[1] != http.StatusConflict {
		t.Fatalf("concurrent move statuses = %v, want [200 409]", statuses)
	}
	childResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/organizations/"+uuid.UUID(child.OrganizationId).String(), nil, adminHeaders)
	assertStatus(t, childResponse, http.StatusOK)
	var movedChild api.OrganizationResponse
	decodeResponse(t, childResponse, &movedChild)
	parentID := uuid.UUID(*movedChild.Organization.ParentOrganizationId)
	parentResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/organizations/"+parentID.String(), nil, adminHeaders)
	assertStatus(t, parentResponse, http.StatusOK)
	var parent api.OrganizationResponse
	decodeResponse(t, parentResponse, &parent)
	cycleResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/organizations/"+parentID.String()+"/move", bytes.NewReader(mustJSON(t, map[string]any{"newParentOrganizationId": uuid.UUID(child.OrganizationId).String(), "rowVersion": parent.Organization.RowVersion, "reason": "must reject cycle"})), adminHeaders)
	assertErrorCode(t, cycleResponse, http.StatusConflict, "ORGANIZATION_TREE_CYCLE")

	planSource := createOrganization("PlanSource", ptrUUID(uuid.UUID(root.OrganizationId)), 1024)
	planHeaders := cloneHeaders(adminHeaders)
	planHeaders["Idempotency-Key"] = "plan-" + suffix
	planResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/organization-change-plans", bytes.NewReader(mustJSON(t, map[string]any{"planType": "MOVE", "name": "Move plan", "expectedTreeVersion": planSource.TreeVersion})), planHeaders)
	assertStatus(t, planResponse, http.StatusCreated)
	var plan api.OrganizationChangePlanResponse
	decodeResponse(t, planResponse, &plan)
	operationHeaders := cloneHeaders(adminHeaders)
	operationHeaders["Idempotency-Key"] = "plan-operation-" + suffix
	operationPayload := mustJSON(t, map[string]any{"sequenceNumber": 1, "operationType": "MOVE_NODE", "sourceOrganizationId": uuid.UUID(planSource.OrganizationId).String(), "targetOrganizationId": uuid.UUID(targetB.OrganizationId).String(), "operationSchemaVersion": 1, "operation": map[string]any{}})
	operationResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/organization-change-plans/"+uuid.UUID(plan.Plan.PlanId).String()+"/operations", bytes.NewReader(operationPayload), operationHeaders)
	assertStatus(t, operationResponse, http.StatusCreated)
	decodeResponse(t, operationResponse, &plan)
	replayOperationResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/organization-change-plans/"+uuid.UUID(plan.Plan.PlanId).String()+"/operations", bytes.NewReader(operationPayload), operationHeaders)
	assertStatus(t, replayOperationResponse, http.StatusCreated)
	decodeResponse(t, replayOperationResponse, &plan)
	if len(plan.Plan.Operations) != 1 {
		t.Fatalf("idempotent operation replay created duplicates: %#v", plan.Plan.Operations)
	}
	for _, action := range []string{"VALIDATE", "APPROVE", "EXECUTE"} {
		transitionResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/organization-change-plans/"+uuid.UUID(plan.Plan.PlanId).String()+"/transition", bytes.NewReader(mustJSON(t, map[string]any{"action": action, "rowVersion": plan.Plan.RowVersion, "reason": "integration plan"})), adminHeaders)
		if transitionResponse.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(transitionResponse.Body)
			_ = transitionResponse.Body.Close()
			t.Fatalf("plan action %s status = %d, want 200; body=%s", action, transitionResponse.StatusCode, body)
		}
		decodeResponse(t, transitionResponse, &plan)
	}
	if plan.Plan.Status != api.OrganizationChangePlanStatusCOMPLETED || len(plan.Plan.Operations) != 1 || plan.Plan.Operations[0].Status != api.OrganizationChangeOperationStatusSUCCESS {
		t.Fatalf("unexpected completed move plan: %#v", plan.Plan)
	}
	plansResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/organization-change-plans?page=1&pageSize=50&status=COMPLETED", nil, adminHeaders)
	assertStatus(t, plansResponse, http.StatusOK)
	var plans api.OrganizationChangePlanListResponse
	decodeResponse(t, plansResponse, &plans)
	if plans.Total < 1 || len(plans.Items) < 1 {
		t.Fatalf("completed plan list is empty: %#v", plans)
	}
	var actualParent uuid.UUID
	if err := connection.QueryRow(ctx, "SELECT parent_organization_id FROM organizations WHERE organization_id = $1", uuid.UUID(planSource.OrganizationId)).Scan(&actualParent); err != nil {
		t.Fatalf("read planned move result: %v", err)
	}
	if actualParent != uuid.UUID(targetB.OrganizationId) {
		t.Fatalf("planned move parent = %s, want %s", actualParent, targetB.OrganizationId)
	}
	pool.Close()

	deleteBlockedResponse := doRequest(t, client, http.MethodPut, baseURL+"/api/v1/admin/organizations/"+uuid.UUID(planSource.OrganizationId).String()+"/status", bytes.NewReader(mustJSON(t, map[string]any{"status": "DELETED", "rowVersion": planSource.RowVersion + 1, "reason": "must remain blocked by organization space"})), adminHeaders)
	assertErrorCode(t, deleteBlockedResponse, http.StatusConflict, "RESOURCE_DELETE_BLOCKED")

	stopServer()
	select {
	case err := <-serverErrors:
		if err != nil {
			t.Fatalf("server shutdown failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not stop within shutdown deadline")
	}
}

func ptrUUID(value uuid.UUID) *uuid.UUID { return &value }

func doRequestNoFatal(client *http.Client, method, url string, payload []byte, headers map[string]string) int {
	request, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		return 0
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}

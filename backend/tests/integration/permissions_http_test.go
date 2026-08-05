package integration_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/app"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/platform/config"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestPermissionsAndDelegationHTTP(t *testing.T) {
	if os.Getenv(integrationEnvironment) != "1" {
		t.Skip("set FILE_WORKSHOP_RUN_INTEGRATION=1 to run local dependency integration tests")
	}
	backendRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FILE_WORKSHOP_ENV_FILE", filepath.Join(backendRoot, ".env"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	connection, err := pgx.Connect(ctx, cfg.PostgreSQL.ConnectionString())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = connection.Exec(ctx, "SET search_path TO "+pgx.Identifier{cfg.PostgreSQL.Schema}.Sanitize()+",public"); err != nil {
		t.Fatal(err)
	}
	migrationBytes, err := os.ReadFile(filepath.Join(backendRoot, "migrations", "00003_fix_namespace_subtype_trigger.sql"))
	if err != nil {
		t.Fatalf("read namespace trigger migration: %v", err)
	}
	migrationUp := strings.SplitN(string(migrationBytes), "-- +goose Down", 2)[0]
	migrationUp = strings.Replace(migrationUp, "-- +goose Up", "", 1)
	if _, err = connection.Exec(ctx, migrationUp); err != nil {
		t.Fatalf("apply namespace trigger fix for integration database: %v", err)
	}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	adminID, delegatedID, granteeID, aclUserID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	userIDs := []uuid.UUID{adminID, delegatedID, granteeID, aclUserID}
	rootOrgID, childOrgID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	organizationIDs := []uuid.UUID{rootOrgID, childOrgID}
	rootSpaceID, childSpaceID, personalSpaceID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	spaceIDs := []uuid.UUID{rootSpaceID, childSpaceID, personalSpaceID}
	folderID, documentID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	delegationIDs, grantIDs := make([]uuid.UUID, 0, 2), make([]uuid.UUID, 0, 3)

	t.Cleanup(func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = connection.Exec(cleanup, "DELETE FROM permission_grant_actions WHERE permission_grant_id = ANY($1::uuid[])", grantIDs)
		_, _ = connection.Exec(cleanup, "DELETE FROM permission_grants WHERE permission_grant_id = ANY($1::uuid[])", grantIDs)
		if len(delegationIDs) > 1 {
			_, _ = connection.Exec(cleanup, "DELETE FROM admin_delegations WHERE admin_delegation_id = $1", delegationIDs[1])
		}
		if len(delegationIDs) > 0 {
			_, _ = connection.Exec(cleanup, "DELETE FROM admin_delegations WHERE admin_delegation_id = $1", delegationIDs[0])
		}
		_, _ = connection.Exec(cleanup, "DELETE FROM outbox_events WHERE aggregate_id = ANY($1::uuid[]) OR aggregate_id = ANY($2::uuid[]) OR aggregate_id = ANY($3::uuid[])", delegationIDs, grantIDs, []uuid.UUID{folderID, documentID})
		_, _ = connection.Exec(cleanup, "DELETE FROM idempotency_records WHERE user_id = ANY($1::uuid[])", userIDs)
		_, _ = connection.Exec(cleanup, "DELETE FROM documents WHERE document_id = $1", documentID)
		_, _ = connection.Exec(cleanup, "DELETE FROM namespace_entries WHERE namespace_entry_id = $1", documentID)
		_, _ = connection.Exec(cleanup, "DELETE FROM folders WHERE folder_id = $1", folderID)
		_, _ = connection.Exec(cleanup, "DELETE FROM namespace_entries WHERE namespace_entry_id = $1", folderID)
		_, _ = connection.Exec(cleanup, "DELETE FROM spaces WHERE space_id = ANY($1::uuid[])", spaceIDs)
		_, _ = connection.Exec(cleanup, "DELETE FROM organization_security_versions WHERE organization_id = ANY($1::uuid[])", organizationIDs)
		_, _ = connection.Exec(cleanup, "DELETE FROM organization_closure WHERE ancestor_organization_id = ANY($1::uuid[]) OR descendant_organization_id = ANY($1::uuid[])", organizationIDs)
		_, _ = connection.Exec(cleanup, "DELETE FROM organizations WHERE organization_id = ANY($1::uuid[])", organizationIDs)
		_, _ = connection.Exec(cleanup, "DELETE FROM login_attempts WHERE username_normalized LIKE $1", "permissions_%_"+suffix)
		_, _ = connection.Exec(cleanup, "DELETE FROM session_refresh_tokens WHERE user_session_id IN (SELECT user_session_id FROM user_sessions WHERE user_id = ANY($1::uuid[]))", userIDs)
		_, _ = connection.Exec(cleanup, "DELETE FROM user_sessions WHERE user_id = ANY($1::uuid[])", userIDs)
		_, _ = connection.Exec(cleanup, "DELETE FROM user_credentials WHERE user_id = ANY($1::uuid[])", userIDs)
		_, _ = connection.Exec(cleanup, "DELETE FROM principal_security_versions WHERE user_id = ANY($1::uuid[])", userIDs)
		_, _ = connection.Exec(cleanup, "DELETE FROM users WHERE user_id = ANY($1::uuid[])", userIDs)
		_ = connection.Close(cleanup)
	})

	hasher := identitydomain.NewArgon2IDHasher()
	type fixture struct {
		id                       uuid.UUID
		username, password, role string
	}
	fixtures := []fixture{{adminID, "permissions_admin_" + suffix, "Admin!Aa1-" + suffix, "SYSTEM_ADMIN"}, {delegatedID, "permissions_delegate_" + suffix, "Delegate!Bb2-" + suffix, "USER"}, {granteeID, "permissions_grantee_" + suffix, "Grantee!Cc3-" + suffix, "USER"}, {aclUserID, "permissions_acl_" + suffix, "AclUser!Dd4-" + suffix, "USER"}}
	now := time.Now().UTC()
	for _, value := range fixtures {
		hash, hashErr := hasher.Hash(value.password)
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		credentialID := uuid.Must(uuid.NewV7())
		if _, err = connection.Exec(ctx, `INSERT INTO users (user_id,username,username_normalized,display_name,system_role,status,locale,timezone,created_at,updated_at) VALUES ($1,$2,$2,$3,$4,'ACTIVE','zh-CN','Asia/Shanghai',$5,$5)`, value.id, value.username, value.username, value.role, now); err != nil {
			t.Fatal(err)
		}
		if _, err = connection.Exec(ctx, `INSERT INTO user_credentials (user_credential_id,user_id,credential_type,identifier,identifier_normalized,secret_hash,status,created_at,updated_at) VALUES ($1,$2,'PASSWORD',$3,$3,$4,'ACTIVE',$5,$5)`, credentialID, value.id, value.username, hash, now); err != nil {
			t.Fatal(err)
		}
		if _, err = connection.Exec(ctx, "INSERT INTO principal_security_versions (user_id,updated_at) VALUES ($1,$2)", value.id, now); err != nil {
			t.Fatal(err)
		}
	}
	fixtureTx, err := connection.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixtureTx.Exec(ctx, `INSERT INTO organizations (organization_id,parent_organization_id,name,normalized_name,code,normalized_code,depth,created_by_user_id,created_at,updated_at) VALUES ($1,NULL,$7,$7,$3,$3,0,$4,$5,$5),($2,$1,$8,$8,$6,$6,1,$4,$5,$5)`, rootOrgID, childOrgID, "ROOT-"+suffix[:8], adminID, now, "CHILD-"+suffix[:8], "permission-root-"+suffix[:8], "permission-child-"+suffix[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err = fixtureTx.Exec(ctx, `INSERT INTO organization_closure (ancestor_organization_id,descendant_organization_id,depth,created_at) VALUES ($1,$1,0,$3),($2,$2,0,$3),($1,$2,1,$3)`, rootOrgID, childOrgID, now); err != nil {
		t.Fatal(err)
	}
	if _, err = fixtureTx.Exec(ctx, "INSERT INTO organization_security_versions (organization_id,updated_at) VALUES ($1,$3),($2,$3)", rootOrgID, childOrgID, now); err != nil {
		t.Fatal(err)
	}
	if _, err = fixtureTx.Exec(ctx, `INSERT INTO spaces (space_id,space_type,name,normalized_name,owner_user_id,organization_id,quota_bytes,created_by_user_id,created_at,updated_at) VALUES ($1,'ORGANIZATION','Root','root',NULL,$4,10000,$6,$7,$7),($2,'ORGANIZATION','Child','child',NULL,$5,10000,$6,$7,$7),($3,'PERSONAL','Personal','personal',$8,NULL,10000,$6,$7,$7)`, rootSpaceID, childSpaceID, personalSpaceID, rootOrgID, childOrgID, adminID, now, aclUserID); err != nil {
		t.Fatal(err)
	}
	if _, err = fixtureTx.Exec(ctx, `INSERT INTO namespace_entries (namespace_entry_id,space_id,parent_folder_id,entry_type,name,normalized_name,depth,created_by_user_id,created_at,updated_at) VALUES ($1,$3,NULL,'FOLDER','Folder','folder',0,$4,$5,$5),($2,$3,$1,'DOCUMENT','Drawing.pdf','drawing.pdf',1,$4,$5,$5)`, folderID, documentID, childSpaceID, aclUserID, now); err != nil {
		t.Fatal(err)
	}
	if _, err = fixtureTx.Exec(ctx, "INSERT INTO folders (folder_id,created_at,updated_at) VALUES ($1,$2,$2)", folderID, now); err != nil {
		t.Fatal(err)
	}
	if _, err = fixtureTx.Exec(ctx, "INSERT INTO documents (document_id,owner_user_id,availability_status,created_at,updated_at) VALUES ($1,$2,'AVAILABLE',$3,$3)", documentID, aclUserID, now); err != nil {
		t.Fatal(err)
	}
	if err = fixtureTx.Commit(ctx); err != nil {
		t.Fatal(err)
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
	loginHeaders := map[uuid.UUID]map[string]string{}
	for _, value := range fixtures {
		login := postLogin(t, client, baseURL, value.username, value.password)
		loginHeaders[value.id] = map[string]string{"Authorization": "Bearer " + login.AccessToken, "Content-Type": "application/json"}
	}

	evaluate := func(userID uuid.UUID, resourceType string, resourceID uuid.UUID, action string, reason *string) api.PermissionEvaluationResult {
		t.Helper()
		body := map[string]any{"resourceType": resourceType, "resourceId": resourceID.String(), "action": action}
		if reason != nil {
			body["privilegedReason"] = *reason
			body["privilegedAccessConfirmed"] = true
		}
		response := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/permissions/evaluate", bytes.NewReader(mustJSON(t, body)), loginHeaders[userID])
		assertStatus(t, response, http.StatusOK)
		var result api.PermissionEvaluationResponse
		decodeResponse(t, response, &result)
		return result.Result
	}
	if result := evaluate(aclUserID, "SPACE", childSpaceID, "DOWNLOAD", nil); result.Allowed {
		t.Fatalf("default deny failed: %#v", result)
	}
	if result := evaluate(aclUserID, "SPACE", personalSpaceID, "DOWNLOAD", nil); !result.Allowed || result.Source != "PERSONAL_OWNER" {
		t.Fatalf("personal owner denied: %#v", result)
	}
	if result := evaluate(adminID, "SPACE", personalSpaceID, "DOWNLOAD", nil); result.Allowed || !result.PrivilegedAccessRequired {
		t.Fatalf("admin privileged reason was not enforced: %#v", result)
	}
	reason := "support ticket FW-1"
	if result := evaluate(adminID, "SPACE", personalSpaceID, "DOWNLOAD", &reason); !result.Allowed {
		t.Fatalf("admin privileged access with reason denied: %#v", result)
	}

	rootHeaders := cloneHeaders(loginHeaders[adminID])
	rootHeaders["Idempotency-Key"] = "delegation-root-" + suffix
	rootResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin-delegations", bytes.NewReader(mustJSON(t, map[string]any{"userId": delegatedID.String(), "organizationId": rootOrgID.String(), "scope": "SUBTREE", "canDelegate": true, "capabilities": []string{"MANAGE_SPACE_CONTENT", "MANAGE_SPACE_PERMISSION", "DELEGATE_ADMIN"}, "validFrom": now.Add(-time.Minute)})), rootHeaders)
	assertStatus(t, rootResponse, http.StatusCreated)
	var rootDelegation api.AdminDelegationResponse
	decodeResponse(t, rootResponse, &rootDelegation)
	delegationIDs = append(delegationIDs, uuid.UUID(rootDelegation.Delegation.DelegationId))
	if result := evaluate(delegatedID, "SPACE", childSpaceID, "DOWNLOAD", nil); !result.Allowed || result.Source != "ADMIN_DELEGATION" {
		t.Fatalf("subtree delegation denied: %#v", result)
	}
	childHeaders := cloneHeaders(loginHeaders[delegatedID])
	childHeaders["Idempotency-Key"] = "delegation-child-" + suffix
	childResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin-delegations", bytes.NewReader(mustJSON(t, map[string]any{"userId": granteeID.String(), "organizationId": childOrgID.String(), "scope": "SELF", "canDelegate": false, "parentDelegationId": delegationIDs[0].String(), "capabilities": []string{"MANAGE_SPACE_CONTENT"}, "validFrom": now})), childHeaders)
	assertStatus(t, childResponse, http.StatusCreated)
	var childDelegation api.AdminDelegationResponse
	decodeResponse(t, childResponse, &childDelegation)
	delegationIDs = append(delegationIDs, uuid.UUID(childDelegation.Delegation.DelegationId))
	if result := evaluate(granteeID, "SPACE", childSpaceID, "DOWNLOAD", nil); !result.Allowed {
		t.Fatalf("child delegation denied: %#v", result)
	}

	grantHeaders := cloneHeaders(loginHeaders[adminID])
	grantHeaders["Idempotency-Key"] = "grant-space-" + suffix
	grantResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/permissions/grants", bytes.NewReader(mustJSON(t, map[string]any{"subjectType": "USER", "subjectId": aclUserID.String(), "resourceType": "SPACE", "resourceId": childSpaceID.String(), "actions": []string{"DOWNLOAD"}, "inheritToDescendants": true, "grantSource": "MANUAL", "validFrom": now.Add(-time.Minute)})), grantHeaders)
	assertStatus(t, grantResponse, http.StatusCreated)
	var spaceGrant api.PermissionGrantResponse
	decodeResponse(t, grantResponse, &spaceGrant)
	grantIDs = append(grantIDs, uuid.UUID(spaceGrant.Grant.GrantId))
	if result := evaluate(aclUserID, "DOCUMENT", documentID, "DOWNLOAD", nil); !result.Allowed || result.Source != "INHERITED_GRANT" {
		t.Fatalf("inherited grant denied after cached default deny: %#v", result)
	}
	breakResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/permissions/resources/DOCUMENT/"+documentID.String()+"/break-inheritance", bytes.NewReader(mustJSON(t, map[string]any{"rowVersion": 1, "reason": "isolate drawing"})), loginHeaders[adminID])
	assertStatus(t, breakResponse, http.StatusOK)
	if result := evaluate(aclUserID, "DOCUMENT", documentID, "DOWNLOAD", nil); result.Allowed {
		t.Fatalf("BREAK inheritance did not invalidate cached allow: %#v", result)
	}
	directHeaders := cloneHeaders(loginHeaders[adminID])
	directHeaders["Idempotency-Key"] = "grant-document-" + suffix
	directResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/permissions/grants", bytes.NewReader(mustJSON(t, map[string]any{"subjectType": "USER", "subjectId": aclUserID.String(), "resourceType": "DOCUMENT", "resourceId": documentID.String(), "actions": []string{"DOWNLOAD"}, "inheritToDescendants": false, "grantSource": "MANUAL", "validFrom": now.Add(-time.Minute)})), directHeaders)
	assertStatus(t, directResponse, http.StatusCreated)
	var directGrant api.PermissionGrantResponse
	decodeResponse(t, directResponse, &directGrant)
	grantIDs = append(grantIDs, uuid.UUID(directGrant.Grant.GrantId))
	if result := evaluate(aclUserID, "DOCUMENT", documentID, "DOWNLOAD", nil); !result.Allowed || result.Source != "DIRECT_GRANT" {
		t.Fatalf("direct grant denied: %#v", result)
	}
	revokeGrant := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/permissions/grants/"+grantIDs[1].String()+"/revoke", bytes.NewReader(mustJSON(t, map[string]any{"rowVersion": directGrant.Grant.RowVersion, "reason": "integration revoke"})), loginHeaders[adminID])
	assertStatus(t, revokeGrant, http.StatusOK)
	if result := evaluate(aclUserID, "DOCUMENT", documentID, "DOWNLOAD", nil); result.Allowed {
		t.Fatalf("revoked direct grant remained effective: %#v", result)
	}

	revokeDelegation := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin-delegations/"+delegationIDs[0].String()+"/revoke", bytes.NewReader(mustJSON(t, map[string]any{"rowVersion": rootDelegation.Delegation.RowVersion, "reason": "integration revoke"})), loginHeaders[adminID])
	assertStatus(t, revokeDelegation, http.StatusOK)
	if result := evaluate(granteeID, "SPACE", childSpaceID, "DOWNLOAD", nil); result.Allowed {
		t.Fatalf("descendant delegation remained effective after parent revoke: %#v", result)
	}
}

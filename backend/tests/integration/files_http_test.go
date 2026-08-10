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

func TestFilesHTTPDirectoryLifecycle(t *testing.T) {
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
	adminID := uuid.Must(uuid.NewV7())
	adminCredentialID := uuid.Must(uuid.NewV7())
	adminUsername := "files_admin_" + suffix
	adminPassword := "Files-" + suffix + "!Aa1"
	spaceIDs := make([]uuid.UUID, 0, 2)

	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = connection.Exec(cleanupContext, "DELETE FROM login_attempts WHERE username_normalized = $1", adminUsername)
		if len(spaceIDs) > 0 {
			cleanupFileSpaces(t, cleanupContext, connection, spaceIDs)
		}
		_, _ = connection.Exec(cleanupContext, "DELETE FROM idempotency_records WHERE user_id = $1", adminID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM session_refresh_tokens WHERE user_session_id IN (SELECT user_session_id FROM user_sessions WHERE user_id = $1)", adminID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM user_sessions WHERE user_id = $1", adminID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM user_credentials WHERE user_id = $1", adminID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM principal_security_versions WHERE user_id = $1", adminID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM users WHERE user_id = $1", adminID)
		_ = connection.Close(cleanupContext)
	})

	hash, err := identitydomain.NewArgon2IDHasher().Hash(adminPassword)
	if err != nil {
		t.Fatalf("hash administrator password: %v", err)
	}
	now := time.Now().UTC()
	if _, err := connection.Exec(ctx, `INSERT INTO users (user_id, username, username_normalized, display_name, system_role, status, locale, timezone, created_at, updated_at) VALUES ($1,$2,$2,$3,'SYSTEM_ADMIN','ACTIVE','zh-CN','Asia/Shanghai',$4,$4)`, adminID, adminUsername, "Files Admin", now); err != nil {
		t.Fatalf("insert admin user fixture: %v", err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO user_credentials (user_credential_id,user_id,credential_type,identifier,identifier_normalized,secret_hash,status,created_at,updated_at) VALUES ($1,$2,'PASSWORD',$3,$3,$4,'ACTIVE',$5,$5)`, adminCredentialID, adminID, adminUsername, hash, now); err != nil {
		t.Fatalf("insert admin credential fixture: %v", err)
	}
	if _, err := connection.Exec(ctx, "INSERT INTO principal_security_versions (user_id, updated_at) VALUES ($1,$2)", adminID, now); err != nil {
		t.Fatalf("insert principal security version fixture: %v", err)
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
	login := postLogin(t, client, baseURL, adminUsername, adminPassword)
	headers := map[string]string{"Authorization": "Bearer " + login.AccessToken, "Content-Type": "application/json"}

	publicHeaders := cloneHeaders(headers)
	publicHeaders["Idempotency-Key"] = "files-public-" + suffix
	publicResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/spaces", bytes.NewReader(mustJSON(t, map[string]any{"name": "工艺资料库", "quotaBytes": int64(4096), "config": map[string]any{"scenario": "files-integration"}})), publicHeaders)
	assertStatus(t, publicResponse, http.StatusCreated)
	var publicSpace api.SpaceResponse
	decodeResponse(t, publicResponse, &publicSpace)
	spaceID := uuid.UUID(publicSpace.Space.SpaceId)
	spaceIDs = append(spaceIDs, spaceID)

	folderHeaders := cloneHeaders(headers)
	folderHeaders["Idempotency-Key"] = "folder-process-" + suffix
	processFolder := createFolderEntry(t, client, baseURL, headers, folderHeaders, spaceID, nil, "工艺文件")
	if processFolder.EntryType != api.DirectoryEntryTypeFOLDER || processFolder.ParentFolderId == nil || processFolder.PathCache == nil || *processFolder.PathCache != "/工艺文件" {
		t.Fatalf("unexpected process folder: %#v", processFolder)
	}

	rootResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/spaces/"+spaceID.String()+"/entries?page=1&pageSize=50", nil, headers)
	assertStatus(t, rootResponse, http.StatusOK)
	var rootList api.DirectoryEntryListResponse
	decodeResponse(t, rootResponse, &rootList)
	if rootList.RootFolderId == nil || len(rootList.Items) != 1 || rootList.Items[0].EntryId != processFolder.EntryId {
		t.Fatalf("unexpected root listing: %#v", rootList)
	}

	documentHeaders := cloneHeaders(headers)
	documentHeaders["Idempotency-Key"] = "doc-sop-001-" + suffix
	documentPayload := map[string]any{"parentFolderId": uuid.UUID(processFolder.EntryId).String(), "name": "SOP-001.pdf", "classification": "INTERNAL", "metadata": map[string]any{"line": "A1"}}
	documentResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/spaces/"+spaceID.String()+"/documents", bytes.NewReader(mustJSON(t, documentPayload)), documentHeaders)
	assertStatus(t, documentResponse, http.StatusCreated)
	var documentResult api.DirectoryEntryResponse
	decodeResponse(t, documentResponse, &documentResult)
	document := documentResult.Entry
	if document.EntryType != api.DirectoryEntryTypeDOCUMENT || document.AvailabilityStatus == nil || *document.AvailabilityStatus != api.BLOCKED || document.ExtensionNormalized == nil || *document.ExtensionNormalized != "pdf" {
		t.Fatalf("unexpected document placeholder: %#v", document)
	}

	childrenURL := baseURL + "/api/v1/spaces/" + spaceID.String() + "/entries?parentFolderId=" + uuid.UUID(processFolder.EntryId).String() + "&page=1&pageSize=50"
	childrenResponse := doRequest(t, client, http.MethodGet, childrenURL, nil, headers)
	assertStatus(t, childrenResponse, http.StatusOK)
	var children api.DirectoryEntryListResponse
	decodeResponse(t, childrenResponse, &children)
	if len(children.Items) != 1 || children.Items[0].EntryId != document.EntryId || children.Items[0].Name != "SOP-001.pdf" {
		t.Fatalf("unexpected folder children: %#v", children)
	}

	renameResponse := doRequest(t, client, http.MethodPatch, baseURL+"/api/v1/entries/"+uuid.UUID(document.EntryId).String(), bytes.NewReader(mustJSON(t, map[string]any{"name": "SOP-002.pdf", "rowVersion": document.RowVersion})), headers)
	assertStatus(t, renameResponse, http.StatusOK)
	decodeResponse(t, renameResponse, &documentResult)
	renamedDocument := documentResult.Entry
	if renamedDocument.Name != "SOP-002.pdf" || renamedDocument.RowVersion <= document.RowVersion || renamedDocument.PathCache == nil || *renamedDocument.PathCache != "/工艺文件/SOP-002.pdf" {
		t.Fatalf("unexpected renamed document: %#v", renamedDocument)
	}

	archiveHeaders := cloneHeaders(headers)
	archiveHeaders["Idempotency-Key"] = "folder-archive-" + suffix
	archiveFolder := createFolderEntry(t, client, baseURL, headers, archiveHeaders, spaceID, nil, "归档")
	moveResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/entries/"+uuid.UUID(renamedDocument.EntryId).String()+"/move", bytes.NewReader(mustJSON(t, map[string]any{"targetParentFolderId": uuid.UUID(archiveFolder.EntryId).String(), "rowVersion": renamedDocument.RowVersion})), headers)
	assertStatus(t, moveResponse, http.StatusOK)
	decodeResponse(t, moveResponse, &documentResult)
	movedDocument := documentResult.Entry
	if movedDocument.ParentFolderId == nil || uuid.UUID(*movedDocument.ParentFolderId) != uuid.UUID(archiveFolder.EntryId) || movedDocument.PathCache == nil || *movedDocument.PathCache != "/归档/SOP-002.pdf" {
		t.Fatalf("unexpected moved document: %#v", movedDocument)
	}

	childHeaders := cloneHeaders(headers)
	childHeaders["Idempotency-Key"] = "folder-process-child-" + suffix
	childFolder := createFolderEntry(t, client, baseURL, headers, childHeaders, spaceID, ptrUUID(uuid.UUID(processFolder.EntryId)), "作业指导书")
	cycleResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/entries/"+uuid.UUID(processFolder.EntryId).String()+"/move", bytes.NewReader(mustJSON(t, map[string]any{"targetParentFolderId": uuid.UUID(childFolder.EntryId).String(), "rowVersion": processFolder.RowVersion})), headers)
	assertErrorCode(t, cycleResponse, http.StatusConflict, "DIRECTORY_TREE_CYCLE")

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

func createFolderEntry(t *testing.T, client *http.Client, baseURL string, defaultHeaders, requestHeaders map[string]string, spaceID uuid.UUID, parentID *uuid.UUID, name string) api.DirectoryEntry {
	t.Helper()
	body := map[string]any{"name": name}
	if parentID != nil {
		body["parentFolderId"] = parentID.String()
	}
	if _, ok := requestHeaders["Content-Type"]; !ok {
		requestHeaders["Content-Type"] = defaultHeaders["Content-Type"]
	}
	response := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/spaces/"+spaceID.String()+"/folders", bytes.NewReader(mustJSON(t, body)), requestHeaders)
	assertStatus(t, response, http.StatusCreated)
	var result api.DirectoryEntryResponse
	decodeResponse(t, response, &result)
	return result.Entry
}

func cleanupFileSpaces(t *testing.T, ctx context.Context, connection *pgx.Conn, spaceIDs []uuid.UUID) {
	t.Helper()
	transaction, err := connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin file cleanup transaction: %v", err)
	}
	defer func() {
		_ = transaction.Rollback(ctx)
	}()
	if _, err := transaction.Exec(ctx, "SET CONSTRAINTS ALL DEFERRED"); err != nil {
		t.Fatalf("defer cleanup constraints: %v", err)
	}
	if _, err := transaction.Exec(ctx, "DELETE FROM outbox_events WHERE aggregate_id = ANY($1::uuid[]) OR aggregate_id IN (SELECT namespace_entry_id FROM namespace_entries WHERE space_id = ANY($1::uuid[]))", spaceIDs); err != nil {
		t.Fatalf("cleanup outbox events: %v", err)
	}
	if _, err := transaction.Exec(ctx, "DELETE FROM quota_reservations WHERE space_id = ANY($1::uuid[])", spaceIDs); err != nil {
		t.Fatalf("cleanup quota reservations: %v", err)
	}
	if _, err := transaction.Exec(ctx, "UPDATE spaces SET root_folder_id = NULL WHERE space_id = ANY($1::uuid[])", spaceIDs); err != nil {
		t.Fatalf("cleanup space root folders: %v", err)
	}
	if _, err := transaction.Exec(ctx, "UPDATE namespace_entries SET parent_folder_id = NULL, depth = 0 WHERE space_id = ANY($1::uuid[])", spaceIDs); err != nil {
		t.Fatalf("cleanup namespace parent folders: %v", err)
	}
	if _, err := transaction.Exec(ctx, "DELETE FROM documents WHERE document_id IN (SELECT namespace_entry_id FROM namespace_entries WHERE space_id = ANY($1::uuid[]))", spaceIDs); err != nil {
		t.Fatalf("cleanup documents: %v", err)
	}
	if _, err := transaction.Exec(ctx, "DELETE FROM folders WHERE folder_id IN (SELECT namespace_entry_id FROM namespace_entries WHERE space_id = ANY($1::uuid[]))", spaceIDs); err != nil {
		t.Fatalf("cleanup folders: %v", err)
	}
	if _, err := transaction.Exec(ctx, "DELETE FROM namespace_entries WHERE space_id = ANY($1::uuid[])", spaceIDs); err != nil {
		t.Fatalf("cleanup namespace entries: %v", err)
	}
	if _, err := transaction.Exec(ctx, "DELETE FROM spaces WHERE space_id = ANY($1::uuid[])", spaceIDs); err != nil {
		t.Fatalf("cleanup spaces: %v", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		t.Fatalf("commit file cleanup transaction: %v", err)
	}
}

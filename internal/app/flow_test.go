package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cng1985/edugo/internal/app"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
)

func TestAccessTokenValidWithoutConfiguredSecret(t *testing.T) {
	engine := bootSecret(t, "empty-secret", false, "")
	admin := login(t, engine, "admin", "AdminPass1")
	postOK(t, engine, "/api/v1/auth/profile", admin.token, map[string]any{})
	refreshed := postOK(t, engine, "/api/v1/auth/refresh", "", map[string]any{"refreshToken": admin.refresh})
	postOK(t, engine, "/api/v1/auth/profile", str(data(refreshed), "accessToken"), map[string]any{})
}

func TestLearningLoop(t *testing.T) {
	engine := boot(t, "loop", false)
	admin := login(t, engine, "admin", "AdminPass1")

	student := createUser(t, engine, admin.token, "stu01", "StudentPass1", "学生一", "student", "default")
	studentB := createUser(t, engine, admin.token, "stu02", "StudentPass1", "学生二", "student", "default")
	postOK(t, engine, "/api/v1/user/disable", admin.token, map[string]any{"userId": studentB.id})
	status, env := post(t, engine, "/api/v1/auth/login", "", map[string]any{"username": "stu02", "password": "StudentPass1"}, nil)
	if status != 401 || str(env, "errorCode") != "ACCOUNT_DISABLED" {
		t.Fatalf("disabled login: %d %v", status, env)
	}
	postOK(t, engine, "/api/v1/user/enable", admin.token, map[string]any{"userId": studentB.id})
	b := login(t, engine, "stu02", "StudentPass1")

	pair := login(t, engine, "stu01", "StudentPass1")
	refreshed := postOK(t, engine, "/api/v1/auth/refresh", "", map[string]any{"refreshToken": pair.refresh})
	status, env = post(t, engine, "/api/v1/auth/refresh", "", map[string]any{"refreshToken": pair.refresh}, nil)
	if status != 401 {
		t.Fatalf("refresh reuse: %d %v", status, env)
	}
	_ = refreshed

	teacher := createUser(t, engine, admin.token, "tea01", "TeacherPass1", "教师一", "teacher", "default")
	tea := login(t, engine, "tea01", "TeacherPass1")

	course := postOK(t, engine, "/api/v1/course/create", tea.token, map[string]any{"title": "Go 入门", "summary": "第一课"})
	courseID := str(data(course), "id")
	version := num(data(course), "version")
	chapter := postOK(t, engine, "/api/v1/chapter/create", tea.token, map[string]any{
		"courseId": courseID, "expectedVersion": version, "title": "基础", "sortOrder": 1,
	})
	version = num(data(chapter), "version")
	chapterID := chapters(data(chapter))[0].id
	lesson1 := postOK(t, engine, "/api/v1/lesson/create", tea.token, map[string]any{
		"courseId": courseID, "chapterId": chapterID, "expectedVersion": version,
		"title": "文档一", "lessonType": "document", "contentText": "第一段", "sortOrder": 1,
	})
	version = num(data(lesson1), "version")
	lesson2 := postOK(t, engine, "/api/v1/lesson/create", tea.token, map[string]any{
		"courseId": courseID, "chapterId": chapterID, "expectedVersion": version,
		"title": "文档二", "lessonType": "document", "contentText": "第二段", "sortOrder": 2,
	})
	version = num(data(lesson2), "version")
	published := postOK(t, engine, "/api/v1/course/publish", tea.token, map[string]any{
		"courseId": courseID, "expectedVersion": version,
	})
	publishedVersion := str(data(published), "publishedVersionId")
	if publishedVersion == "" || str(data(published), "status") != "published" {
		t.Fatalf("publish: %v", data(published))
	}
	edited := postOK(t, engine, "/api/v1/course/update", tea.token, map[string]any{
		"courseId": courseID, "expectedVersion": num(data(published), "version"), "title": "Go 入门修订",
	})
	if str(data(edited), "publishedVersionId") != publishedVersion || str(data(edited), "status") != "published" {
		t.Fatalf("published version changed: %v", data(edited))
	}
	lessons := lessonsOf(data(published))
	if len(lessons) != 2 {
		t.Fatalf("lessons: %v", data(published))
	}

	stu := login(t, engine, "stu01", "StudentPass1")
	postOK(t, engine, "/api/v1/enrollment/join", stu.token, map[string]any{"courseId": courseID})
	sess := postOK(t, engine, "/api/v1/studyRecord/startSession", stu.token, map[string]any{"lessonId": lessons[0]})
	sessionID := str(data(sess), "id")
	first := postOK(t, engine, "/api/v1/studyRecord/submit", stu.token, map[string]any{
		"sessionId": sessionID, "sequence": 1, "recordType": "confirm",
	})
	dup := postOK(t, engine, "/api/v1/studyRecord/submit", stu.token, map[string]any{
		"sessionId": sessionID, "sequence": 1, "recordType": "confirm",
	})
	if data(dup)["duplicate"] != true {
		t.Fatalf("heartbeat dedup: %v", data(dup))
	}
	if num(data(first), "effectiveSec") != num(data(dup), "effectiveSec") {
		t.Fatalf("dedup changed time: %v %v", data(first), data(dup))
	}
	progress := postOK(t, engine, "/api/v1/progress/get", stu.token, map[string]any{"courseId": courseID})
	if num(data(progress), "rate") != 50 || num(data(progress), "requiredDone") != 1 || num(data(progress), "requiredTotal") != 2 {
		t.Fatalf("progress: %v", data(progress))
	}

	q := postOK(t, engine, "/api/v1/question/create", tea.token, map[string]any{
		"type": "single", "stem": "1+1", "difficulty": "easy", "maxScoreCenti": 1000,
		"options": []any{map[string]any{"key": "A", "text": "2"}, map[string]any{"key": "B", "text": "3"}},
		"answer":  map[string]any{"choice": "A"},
	})
	qID := str(data(q), "id")
	qVer := str(data(q), "versionId")
	reviewed := postOK(t, engine, "/api/v1/question/submitReview", tea.token, map[string]any{"questionId": qID, "expectedVersion": 1})
	postOK(t, engine, "/api/v1/question/approve", tea.token, map[string]any{"questionId": qID, "expectedVersion": num(data(reviewed), "version")})

	paper := postOK(t, engine, "/api/v1/paper/create", tea.token, map[string]any{"title": "小测"})
	paperID := str(data(paper), "id")
	updated := postOK(t, engine, "/api/v1/paper/update", tea.token, map[string]any{
		"paperId": paperID, "title": "小测", "expectedVersion": 1, "questionVersionIds": []string{qVer}, "scores": []int{1000},
	})
	pubPaper := postOK(t, engine, "/api/v1/paper/publish", tea.token, map[string]any{"paperId": paperID, "expectedVersion": num(data(updated), "version")})
	exam := postOK(t, engine, "/api/v1/exam/create", tea.token, map[string]any{
		"title": "小测", "paperId": paperID, "durationSec": 1800, "maxAttempts": 2, "passScoreCenti": 600, "publishMode": "manual",
	})
	examID := str(data(exam), "id")
	postOK(t, engine, "/api/v1/exam/publish", tea.token, map[string]any{"examId": examID, "expectedVersion": num(data(exam), "version")})
	_ = pubPaper

	status, env = post(t, engine, "/api/v1/exam/start", stu.token, map[string]any{"examId": examID}, nil)
	if status != 400 {
		t.Fatalf("start without key: %d %v", status, env)
	}
	startStatus, startEnv := post(t, engine, "/api/v1/exam/start", stu.token, map[string]any{"examId": examID}, map[string]string{"Idempotency-Key": "start-1"})
	if startStatus != 200 {
		t.Fatalf("start: %d %v", startStatus, startEnv)
	}
	_, againEnv := post(t, engine, "/api/v1/exam/start", stu.token, map[string]any{"examId": examID}, map[string]string{"Idempotency-Key": "start-1"})
	if str(data(againEnv), "id") != str(data(startEnv), "id") {
		t.Fatalf("idempotent start: %v %v", data(startEnv), data(againEnv))
	}
	attemptID := str(data(startEnv), "id")
	questions := asMaps(data(startEnv)["questions"])
	if len(questions) != 1 {
		t.Fatalf("questions: %v", data(startEnv))
	}
	qid := str(questions[0], "id")
	saved := postOK(t, engine, "/api/v1/exam/saveAnswer", stu.token, map[string]any{
		"attemptId": attemptID, "questionId": qid, "expectedVersion": 0, "answer": map[string]any{"choice": "A"},
	})
	if num(data(saved), "version") != 1 {
		t.Fatalf("answer version: %v", data(saved))
	}
	status, env = post(t, engine, "/api/v1/exam/saveAnswer", stu.token, map[string]any{
		"attemptId": attemptID, "questionId": qid, "expectedVersion": 0, "answer": map[string]any{"choice": "B"},
	}, nil)
	if status != 409 || str(env, "errorCode") != "ATTEMPT_VERSION_CONFLICT" {
		t.Fatalf("version conflict: %d %v", status, env)
	}
	submitted := postOK(t, engine, "/api/v1/exam/submit", stu.token, map[string]any{"attemptId": attemptID})
	if str(data(submitted), "status") != "ready" {
		t.Fatalf("submit status: %v", data(submitted))
	}
	if _, ok := data(submitted)["score"]; ok {
		t.Fatalf("unpublished score leaked on submit: %v", data(submitted))
	}
	againSubmit := postOK(t, engine, "/api/v1/exam/submit", stu.token, map[string]any{"attemptId": attemptID})
	if str(data(againSubmit), "status") != "ready" {
		t.Fatalf("second submit: %v", data(againSubmit))
	}
	result := postOK(t, engine, "/api/v1/exam/result", stu.token, map[string]any{"attemptId": attemptID})
	if _, ok := data(result)["score"]; ok {
		t.Fatalf("student sees unpublished score: %v", data(result))
	}
	status, env = post(t, engine, "/api/v1/exam/result", b.token, map[string]any{"attemptId": attemptID}, nil)
	if status != 404 {
		t.Fatalf("student B result: %d %v", status, env)
	}

	card := postOK(t, engine, "/api/v1/reviewCard/create", stu.token, map[string]any{"front": "1+1", "back": "2"})
	reviewedCard := postOK(t, engine, "/api/v1/reviewCard/submit", stu.token, map[string]any{
		"cardId": str(data(card), "id"), "submissionId": "sub-1", "quality": 4, "expectedVersion": num(data(card), "version"),
	})
	if num(data(reviewedCard), "intervalDays") != 1 || num(data(reviewedCard), "repetitions") != 1 {
		t.Fatalf("sm2: %v", data(reviewedCard))
	}

	sess2 := postOK(t, engine, "/api/v1/studyRecord/startSession", stu.token, map[string]any{"lessonId": lessons[1]})
	postOK(t, engine, "/api/v1/studyRecord/submit", stu.token, map[string]any{
		"sessionId": str(data(sess2), "id"), "sequence": 1, "recordType": "confirm",
	})
	done := postOK(t, engine, "/api/v1/progress/get", stu.token, map[string]any{"courseId": courseID})
	if data(done)["completed"] != true {
		t.Fatalf("course not completed: %v", data(done))
	}
	cert := postOK(t, engine, "/api/v1/certificate/issue", stu.token, map[string]any{"courseId": courseID})
	cert2 := postOK(t, engine, "/api/v1/certificate/issue", stu.token, map[string]any{"courseId": courseID})
	if str(data(cert), "id") == "" || str(data(cert), "id") != str(data(cert2), "id") {
		t.Fatalf("certificate unique: %v %v", data(cert), data(cert2))
	}

	status, env = post(t, engine, "/api/v1/query/search", stu.token, map[string]any{
		"model": "course", "page": 1, "size": 20,
		"filters": map[string]any{"title": map[string]any{"op": "eq", "value": "Go'; drop table courses;--"}},
	}, nil)
	if status != 400 {
		t.Fatalf("sql fragment: %d %v", status, env)
	}
	one := postOK(t, engine, "/api/v1/query/one", stu.token, map[string]any{
		"model": "course", "filters": map[string]any{"title": map[string]any{"op": "eq", "value": "不存在的课程"}},
	})
	if one["data"] != nil {
		t.Fatalf("one empty: %v", one)
	}
	_ = student
	_ = teacher
}

func TestOrganizationIsolation(t *testing.T) {
	engine := boot(t, "org", true)
	admin := login(t, engine, "admin", "AdminPass1")
	createUser(t, engine, admin.token, "teaA", "TeacherPass1", "教师A", "teacher", "org-a")
	createUser(t, engine, admin.token, "teaB", "TeacherPass1", "教师B", "teacher", "org-b")
	a := login(t, engine, "teaA", "TeacherPass1")
	b := login(t, engine, "teaB", "TeacherPass1")
	course := postOK(t, engine, "/api/v1/course/create", a.token, map[string]any{"title": "机构A课程", "summary": "仅A"})
	if str(data(course), "orgId") != "org-a" {
		t.Fatalf("org: %v", data(course))
	}
	listed := postOK(t, engine, "/api/v1/course/list", b.token, map[string]any{"page": 1, "size": 20})
	if num(listed, "total") != 0 {
		t.Fatalf("org leak: %v", listed["data"])
	}
	status, env := post(t, engine, "/api/v1/organization/create", admin.token, map[string]any{"name": "机构A"}, nil)
	if status != 200 {
		t.Fatalf("org create: %d %v", status, env)
	}
}

type session struct {
	token   string
	refresh string
}

type chapterRef struct{ id string }

func boot(t *testing.T, name string, org bool) *gin.Engine {
	t.Helper()
	return bootSecret(t, name, org, "integration-test-secret-32chars-min")
}

func bootSecret(t *testing.T, name string, org bool, jwtSecret string) *gin.Engine {
	t.Helper()
	if jwtSecret == "" {
		t.Setenv("EDUGO_AUTH_JWT_SECRET", "")
	}
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", name)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	orgFlag := "false"
	if org {
		orgFlag = "true"
	}
	body := fmt.Sprintf(`server:
  port: 18080
  mode: test
database:
  driver: sqlite
  dsn: "%s"
  maxOpenConns: 1
  maxIdleConns: 1
auth:
  jwtSecret: %q
scheduler:
  enabled: false
  timezone: Asia/Shanghai
  checkInterval: 1m
eventbus:
  pollInterval: 1h
organization:
  enabled: %s
storage:
  root: %q
admin:
  username: admin
  password: AdminPass1
security:
  bcryptCost: 4
`, dsn, jwtSecret, orgFlag, filepath.Join(dir, "uploads"))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDUGO_CONFIG_PATH", path)
	t.Setenv("EDUGO_SERVER_MODE", "test")
	t.Setenv("EDUGO_SEED_DEMO", "false")
	t.Setenv("EDUGO_AI_ENABLED", "false")
	var srv *httpserver.Server
	application := fx.New(append(app.Options(), fx.Populate(&srv), fx.NopLogger)...)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := application.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		stop, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = application.Stop(stop)
	})
	return srv.Engine
}

func login(t *testing.T, engine *gin.Engine, user, pass string) session {
	t.Helper()
	env := postOK(t, engine, "/api/v1/auth/login", "", map[string]any{"username": user, "password": pass})
	return session{token: str(data(env), "accessToken"), refresh: str(data(env), "refreshToken")}
}

func createUser(t *testing.T, engine *gin.Engine, token, user, pass, name, role, org string) struct{ id string } {
	t.Helper()
	env := postOK(t, engine, "/api/v1/user/create", token, map[string]any{
		"username": user, "password": pass, "displayName": name, "role": role, "orgId": org,
	})
	return struct{ id string }{id: str(data(env), "id")}
}

func postOK(t *testing.T, engine *gin.Engine, path, token string, body any) map[string]any {
	t.Helper()
	status, env := post(t, engine, path, token, body, nil)
	if status != 200 {
		t.Fatalf("%s: %d %v", path, status, env)
	}
	return env
}

func post(t *testing.T, engine *gin.Engine, path, token string, body any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	var env map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("%s decode %s: %v", path, rec.Body.String(), err)
		}
	}
	return rec.Code, env
}

func data(env map[string]any) map[string]any {
	d, _ := env["data"].(map[string]any)
	return d
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func num(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}

func chapters(m map[string]any) []chapterRef {
	raw, _ := m["chapters"].([]any)
	out := make([]chapterRef, 0, len(raw))
	for _, item := range raw {
		row, _ := item.(map[string]any)
		out = append(out, chapterRef{id: str(row, "id")})
	}
	return out
}

func lessonsOf(m map[string]any) []string {
	raw, _ := m["chapters"].([]any)
	var ids []string
	for _, item := range raw {
		row, _ := item.(map[string]any)
		lessons, _ := row["lessons"].([]any)
		for _, lesson := range lessons {
			l, _ := lesson.(map[string]any)
			ids = append(ids, str(l, "id"))
		}
	}
	return ids
}

func asMaps(v any) []map[string]any {
	raw, _ := v.([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		row, _ := item.(map[string]any)
		out = append(out, row)
	}
	return out
}

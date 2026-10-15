package tutoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/platform/llm"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/trace"
	"github.com/cng1985/edugo/internal/shared/xid"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

type Session struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	UserID    string    `gorm:"size:36;index" json:"userId"`
	LessonID  string    `gorm:"size:36" json:"lessonId"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	Deleted   bool      `json:"-"`
}

func (Session) TableName() string { return "ai_chat_sessions" }

type Message struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	SessionID string    `gorm:"size:36;not null;uniqueIndex:uk_msg" json:"sessionId"`
	Role      string    `gorm:"size:16;not null;uniqueIndex:uk_msg" json:"role"`
	RequestID string    `gorm:"size:36;not null;uniqueIndex:uk_msg" json:"requestId"`
	Status    string    `gorm:"size:32" json:"status"`
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

func (Message) TableName() string { return "ai_chat_messages" }

type Service struct {
	db  *gorm.DB
	llm llm.Client
}

func New(db *gorm.DB, client llm.Client) *Service { return &Service{db: db, llm: client} }

func (s *Service) Create(ctx context.Context, lessonID, title string) (Session, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Session{}, err
	}
	row := Session{ID: xid.New(), UserID: p.UserID, LessonID: lessonID, Title: title, CreatedAt: time.Now().UTC()}
	return row, s.db.Create(&row).Error
}

func (s *Service) owned(ctx context.Context, id string) (Session, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Session{}, err
	}
	var row Session
	if err := s.db.Where("id = ? AND user_id = ? AND deleted = ?", id, p.UserID, false).First(&row).Error; err != nil {
		return Session{}, apperr.NotFound("会话不存在")
	}
	return row, nil
}

func (s *Service) History(ctx context.Context, id string) ([]Message, error) {
	if _, err := s.owned(ctx, id); err != nil {
		return nil, err
	}
	var rows []Message
	err := s.db.Where("session_id = ?", id).Order("created_at asc").Find(&rows).Error
	return rows, err
}

func (s *Service) Delete(ctx context.Context, id string) error {
	row, err := s.owned(ctx, id)
	if err != nil {
		return err
	}
	row.Deleted = true
	return s.db.Save(&row).Error
}

func (s *Service) Send(ctx context.Context, sessionID, requestID, content string, onDelta func(string) error) (Message, error) {
	if _, err := s.owned(ctx, sessionID); err != nil {
		return Message{}, err
	}
	if len(content) > 8000 {
		return Message{}, apperr.Invalid("提问超过 8000 字符")
	}
	if requestID == "" {
		requestID = xid.New()
	}
	var existing Message
	if err := s.db.Where("session_id = ? AND request_id = ? AND role = ?", sessionID, requestID, "assistant").First(&existing).Error; err == nil {
		return existing, nil
	}
	userMsg := Message{ID: xid.New(), SessionID: sessionID, Role: "user", RequestID: requestID, Status: "completed", Content: content, CreatedAt: time.Now().UTC()}
	assistant := Message{ID: xid.New(), SessionID: sessionID, Role: "assistant", RequestID: requestID, Status: "streaming", CreatedAt: time.Now().UTC()}
	if err := s.db.Create(&userMsg).Error; err != nil {
		return Message{}, err
	}
	if err := s.db.Create(&assistant).Error; err != nil {
		return Message{}, err
	}
	stream, err := s.llm.Stream(ctx, llm.ChatRequest{
		Module: "tutoring", TaskID: assistant.ID, Profile: "tutoring", PromptVersion: "tutor-v1",
		System: "你是学习助教。如果没有提供课程证据，必须说明信息不足，不能把猜测说成课程事实。",
		User:   content,
	})
	if err != nil {
		assistant.Status = "failed"
		assistant.Content = ""
		_ = s.db.Save(&assistant).Error
		return assistant, err
	}
	defer stream.Close()
	var buf string
	for {
		delta, done, _, recErr := stream.Recv(ctx)
		if recErr != nil {
			status := "failed"
			if ctx.Err() != nil {
				status = "cancelled"
			}
			assistant.Status = status
			assistant.Content = buf
			_ = s.db.Save(&assistant).Error
			return assistant, recErr
		}
		if delta != "" {
			buf += delta
			if onDelta != nil {
				if err := onDelta(delta); err != nil {
					assistant.Status = "cancelled"
					assistant.Content = buf
					_ = s.db.Save(&assistant).Error
					return assistant, err
				}
			}
		}
		if done {
			break
		}
	}
	if buf == "" {
		buf = "当前没有足够的课程证据，无法确认这个问题。"
	}
	assistant.Status = "completed"
	assistant.Content = buf
	return assistant, s.db.Save(&assistant).Error
}

type Handler struct{ svc *Service }

func newHandler(s *Service) *Handler { return &Handler{s} }

func (h *Handler) Register(_, g *gin.RouterGroup) {
	g.POST("/aiChat/createSession", func(c *gin.Context) {
		var req struct {
			LessonID string `json:"lessonId"`
			Title    string `json:"title"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		out, err := h.svc.Create(c.Request.Context(), req.LessonID, req.Title)
		write(c, out, err)
	})
	g.POST("/aiChat/history", func(c *gin.Context) {
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		out, err := h.svc.History(c.Request.Context(), req.SessionID)
		write(c, out, err)
	})
	g.POST("/aiChat/deleteSession", func(c *gin.Context) {
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		write(c, gin.H{"deleted": true}, h.svc.Delete(c.Request.Context(), req.SessionID))
	})
	g.POST("/aiChat/messageStatus", func(c *gin.Context) {
		var req struct {
			SessionID string `json:"sessionId"`
			RequestID string `json:"requestId"`
		}
		if err := httpserver.Bind(c, &req); err != nil {
			httpserver.Fail(c, err)
			return
		}
		var row Message
		if err := h.svc.db.Where("session_id = ? AND request_id = ? AND role = ?", req.SessionID, req.RequestID, "assistant").First(&row).Error; err != nil {
			httpserver.Fail(c, apperr.NotFound("消息不存在"))
			return
		}
		if _, err := h.svc.owned(c.Request.Context(), req.SessionID); err != nil {
			httpserver.Fail(c, err)
			return
		}
		httpserver.OK(c, row)
	})
	g.POST("/aiChat/sendMessage", h.send)
	g.POST("/aiQuestion/generateExplanation", h.send)
}

func (h *Handler) send(c *gin.Context) {
	var req struct {
		SessionID string `json:"sessionId"`
		Content   string `json:"content"`
	}
	if err := httpserver.Bind(c, &req); err != nil {
		httpserver.Fail(c, err)
		return
	}
	requestID := c.GetHeader("Idempotency-Key")
	if c.GetHeader("Accept") != "text/event-stream" {
		out, err := h.svc.Send(c.Request.Context(), req.SessionID, requestID, req.Content, nil)
		write(c, out, err)
		return
	}
	var deltas []string
	msg, err := h.svc.Send(c.Request.Context(), req.SessionID, requestID, req.Content, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil && msg.ID == "" {
		httpserver.Fail(c, err)
		return
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	flusher, _ := c.Writer.(http.Flusher)
	writeEvent := func(name, payload string) {
		fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", name, payload)
		if flusher != nil {
			flusher.Flush()
		}
	}
	started, _ := json.Marshal(map[string]string{"requestId": msg.RequestID, "messageId": msg.ID})
	writeEvent("started", string(started))
	for _, delta := range deltas {
		raw, _ := json.Marshal(map[string]any{"content": delta})
		writeEvent("delta", string(raw))
	}
	if err != nil || msg.Status != "completed" {
		raw, _ := json.Marshal(map[string]any{"errorCode": apperr.As(err).ErrorCode, "retryable": true, "traceId": trace.From(c.Request.Context())})
		writeEvent("error", string(raw))
		return
	}
	done, _ := json.Marshal(map[string]any{"status": "completed"})
	writeEvent("completed", string(done))
}

func write(c *gin.Context, data any, err error) {
	if err != nil {
		httpserver.Fail(c, err)
		return
	}
	httpserver.OK(c, data)
}

var Module = fx.Module("tutoring",
	fx.Provide(fx.Private, newHandler),
	fx.Provide(New,
		fx.Annotate(func(h *Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution { return database.Contribution{Models: []any{&Session{}, &Message{}}} }, fx.ResultTags(`group:"schemas"`)),
	),
)

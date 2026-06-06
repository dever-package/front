package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/server"
)

const (
	TransportSSE    = "sse"
	defaultSSEBlock = 15 * time.Second
	defaultSSECount = int64(20)
)

type Reader func(ctx context.Context, requestID string, lastID string, count int64, block time.Duration) ([]Entry, error)

type ReadParams struct {
	RequestID string
	LastID    string
	Count     int64
	Block     time.Duration
}

var sseEventNamePattern = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func ReadParamsFromServerContext(c *server.Context) ReadParams {
	return NormalizeReadParams(
		firstServerInputText(c, "request_id", "requestId"),
		firstServerInputText(c, "last_id", "lastId"),
		InputInt64(c.Input("count"), 1),
		time.Duration(InputInt64(c.Input("block"), 0))*time.Millisecond,
	)
}

func NormalizeReadParams(requestID string, lastID string, count int64, block time.Duration) ReadParams {
	requestID = strings.TrimSpace(requestID)
	lastID = strings.TrimSpace(lastID)
	if lastID == "" {
		lastID = "0-0"
	}
	if count <= 0 {
		count = 1
	}
	if count > defaultSSECount {
		count = defaultSSECount
	}
	return ReadParams{
		RequestID: requestID,
		LastID:    lastID,
		Count:     count,
		Block:     block,
	}
}

func WantsSSE(c *server.Context) bool {
	if strings.EqualFold(strings.TrimSpace(c.Input("transport")), TransportSSE) {
		return true
	}
	fiberCtx, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return false
	}
	return strings.Contains(strings.ToLower(fiberCtx.Get("Accept")), "text/event-stream")
}

func ServeSSE(c *server.Context, reader Reader, params ReadParams) error {
	fiberCtx, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return fmt.Errorf("SSE: not supported framework")
	}
	params = NormalizeReadParams(params.RequestID, params.LastID, params.Count, params.Block)
	ctx := c.Context()

	fiberCtx.Status(fiber.StatusOK)
	fiberCtx.Set("Content-Type", "text/event-stream; charset=utf-8")
	fiberCtx.Set("Cache-Control", "no-cache, no-transform")
	fiberCtx.Set("Connection", "keep-alive")
	fiberCtx.Set("X-Accel-Buffering", "no")
	fiberCtx.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		serveSSELoop(ctx, w, reader, params)
	})
	return nil
}

func serveSSELoop(ctx context.Context, w *bufio.Writer, reader Reader, params ReadParams) {
	if params.RequestID == "" {
		_ = writeSSEPayload(w, "", ResponsePayload("", "result", map[string]any{}, "request_id 不能为空", 2))
		return
	}

	lastID := params.LastID
	block := params.Block
	if block <= 0 {
		block = defaultSSEBlock
	}
	count := params.Count
	if count <= 0 {
		count = defaultSSECount
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		entries, err := reader(ctx, params.RequestID, lastID, count, block)
		if err != nil {
			_ = writeSSEPayload(w, "", ResponsePayload(params.RequestID, "result", map[string]any{}, err.Error(), 2))
			return
		}
		if len(entries) == 0 {
			if err := writeSSEComment(w, "keepalive"); err != nil {
				return
			}
			continue
		}

		for _, entry := range entries {
			payload := NextPayload(params.RequestID, lastID, []Entry{entry})
			if entry.ID != "" {
				lastID = entry.ID
			}
			if err := writeSSEPayload(w, entry.ID, payload); err != nil {
				return
			}
			if IsResultPayload(payload) {
				return
			}
		}
	}
}

func IsResultPayload(payload map[string]any) bool {
	return strings.EqualFold(InputText(payload["type"]), "result")
}

func writeSSEPayload(w *bufio.Writer, id string, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", sanitizeSSELine(id)); err != nil {
			return err
		}
	}
	if event := sseEventName(payload); event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return err
	}
	return w.Flush()
}

func writeSSEComment(w *bufio.Writer, text string) error {
	if _, err := fmt.Fprintf(w, ": %s\n\n", sanitizeSSELine(text)); err != nil {
		return err
	}
	return w.Flush()
}

func sseEventName(payload map[string]any) string {
	output, _ := payload["output"].(map[string]any)
	event := strings.TrimSpace(InputText(output["event"]))
	if event == "" {
		event = strings.TrimSpace(InputText(payload["type"]))
	}
	event = sseEventNamePattern.ReplaceAllString(event, "_")
	return strings.Trim(event, "_.-")
}

func sanitizeSSELine(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(strings.TrimSpace(value))
}

func firstServerInputText(c *server.Context, keys ...string) string {
	for _, key := range keys {
		if text := strings.TrimSpace(InputText(c.Input(key))); text != "" {
			return text
		}
	}
	return ""
}

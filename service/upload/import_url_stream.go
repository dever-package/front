package upload

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shemic/dever/server"

	frontstream "my/package/front/service/stream"
	uploadrepo "my/package/front/service/upload/repository"
)

const uploadImportStreamTimeout = 15 * time.Minute

var uploadStreams = frontstream.New("upload")

type uploadImportURLStreamInput struct {
	uploadImportURLInput
	RequestID string `json:"request_id"`
}

type uploadStreamWriter struct {
	requestID    string
	lastProgress int
	lastWritten  time.Time
}

func ImportURLUploadStream(c *server.Context) error {
	var input uploadImportURLStreamInput
	if err := c.BindJSON(&input); err != nil {
		return c.Error("请求体格式错误")
	}

	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		requestID = uuid.NewString()
	}

	startPayload := frontstream.ResponsePayload(requestID, "stream", map[string]any{
		"event": "start",
		"text":  "资源保存任务已开始",
		"meta": map[string]any{
			"stream_key": uploadStreams.Key(requestID),
		},
	}, "", 1)
	streamID, err := uploadStreams.WritePayload(c.Context(), requestID, startPayload)
	if err != nil {
		return c.Error(err)
	}
	startPayload["stream_id"] = streamID

	go runImportURLUploadStream(requestID, input.uploadImportURLInput)
	return c.JSONPayload(200, startPayload)
}

func ReadUploadStream(c *server.Context) error {
	params := frontstream.ReadParamsFromServerContext(c)
	if frontstream.WantsSSE(c) {
		return frontstream.ServeSSE(c, uploadStreams.Read, params)
	}

	entries, err := uploadStreams.Read(c.Context(), params.RequestID, params.LastID, params.Count, params.Block)
	if err != nil {
		return c.JSONPayload(200, frontstream.ResponsePayload(params.RequestID, "result", map[string]any{}, err.Error(), 2))
	}
	return c.JSONPayload(200, frontstream.NextPayload(params.RequestID, params.LastID, entries))
}

func runImportURLUploadStream(requestID string, input uploadImportURLInput) {
	ctx, cancel := context.WithTimeout(context.Background(), uploadImportStreamTimeout)
	defer cancel()

	writer := newUploadStreamWriter(requestID)
	fileRecord, err := importURLUploadWithProgress(ctx, input, writer.Progress)
	if err != nil {
		writer.Error(err)
		return
	}
	payload := uploadrepo.BuildUploadFilePayload(fileRecord)
	writer.Progress("资源保存完成", 100)
	writer.Result(input.Kind, payload)
}

func importURLUploadWithProgress(
	ctx context.Context,
	input uploadImportURLInput,
	progress func(text string, progress int),
) (resolvedUploadFile, error) {
	rule, err := uploadrepo.FindUploadRule(ctx, input.RuleID)
	if err != nil {
		return resolvedUploadFile{}, err
	}
	notifyImportURLProgress(progress, "正在准备保存资源", 2)

	localPath, name, mimeType, cleanup, err := downloadImportURLFile(ctx, input, uploadRuleMaxSizeBytes(rule), progress)
	if err != nil {
		return resolvedUploadFile{}, err
	}
	defer cleanup()

	fileRecord, err := importFileWithRule(ctx, ImportFileInput{
		RuleID:     input.RuleID,
		Kind:       input.Kind,
		Name:       name,
		Mime:       mimeType,
		LocalPath:  localPath,
		BizKey:     input.BizKey,
		BizName:    input.BizName,
		CategoryID: input.CategoryID,
		Progress:   progress,
	}, rule)
	if err != nil {
		return resolvedUploadFile{}, err
	}
	return fileRecord, nil
}

func newUploadStreamWriter(requestID string) *uploadStreamWriter {
	return &uploadStreamWriter{
		requestID:    strings.TrimSpace(requestID),
		lastProgress: -1,
	}
}

func (w *uploadStreamWriter) Progress(text string, progress int) {
	if w == nil || w.requestID == "" {
		return
	}
	if !w.shouldWrite(progress) {
		return
	}
	w.write(frontstream.ProgressPayload(w.requestID, text, progress))
}

func (w *uploadStreamWriter) Result(kind string, file map[string]any) {
	if w == nil || w.requestID == "" {
		return
	}
	output := map[string]any{
		"event":    "result",
		"resource": file,
	}
	if key := uploadOutputKey(kind, file); key != "" {
		output[key] = []any{file["url"]}
	}
	w.write(frontstream.ResponsePayload(w.requestID, "result", output, "", 1))
}

func (w *uploadStreamWriter) Error(err error) {
	if w == nil || w.requestID == "" || err == nil {
		return
	}
	w.write(frontstream.ResponsePayload(w.requestID, "result", map[string]any{}, err.Error(), 2))
}

func (w *uploadStreamWriter) shouldWrite(progress int) bool {
	now := time.Now()
	if progress >= 0 {
		if progress > 100 {
			progress = 100
		}
		if progress < w.lastProgress {
			return false
		}
		if progress == w.lastProgress && now.Sub(w.lastWritten) < 500*time.Millisecond {
			return false
		}
		w.lastProgress = progress
	}
	w.lastWritten = now
	return true
}

func (w *uploadStreamWriter) write(payload map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = uploadStreams.WritePayload(ctx, w.requestID, payload)
}

func uploadOutputKey(kind string, file map[string]any) string {
	normalizedKind := strings.ToLower(strings.TrimSpace(firstNonEmpty(kind, frontstream.InputText(file["kind"]))))
	switch normalizedKind {
	case "image", "images":
		return "images"
	case "video", "videos":
		return "videos"
	case "audio", "audios":
		return "audios"
	case "file", "files":
		return "files"
	default:
		return "files"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

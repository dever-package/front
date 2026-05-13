package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultMaxLen = int64(2000)
	defaultTTL    = 24 * time.Hour
)

type Service struct {
	namespace     string
	client        *redis.Client
	maxLen        int64
	ttl           time.Duration
	memory        *memoryStreamStore
	redisDisabled *atomic.Bool
}

type Entry struct {
	ID      string         `json:"id"`
	Payload map[string]any `json:"payload"`
}

func New(namespace string) Service {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = "default"
	}
	envPrefix := strings.ToUpper(strings.ReplaceAll(namespace, "-", "_"))
	return Service{
		namespace:     namespace,
		client:        newRedisClientFromEnv(envPrefix),
		maxLen:        envInt64(envPrefix+"_STREAM_MAXLEN", envInt64("STREAM_MAXLEN", defaultMaxLen)),
		ttl:           envDuration(envPrefix+"_STREAM_TTL", envDuration("STREAM_TTL", defaultTTL)),
		memory:        newMemoryStreamStore(envInt64(envPrefix+"_STREAM_MAXLEN", envInt64("STREAM_MAXLEN", defaultMaxLen)), envDuration(envPrefix+"_STREAM_TTL", envDuration("STREAM_TTL", defaultTTL))),
		redisDisabled: &atomic.Bool{},
	}
}

func (s Service) WritePayload(ctx context.Context, requestID string, payload map[string]any) (string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", fmt.Errorf("request_id 不能为空")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if strings.TrimSpace(InputText(payload["request_id"])) == "" {
		payload["request_id"] = requestID
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	streamKey := s.Key(requestID)
	if s.canUseRedis() {
		id, err := s.client.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey,
			MaxLen: s.maxLen,
			Approx: true,
			Values: map[string]any{
				"request_id": requestID,
				"type":       strings.TrimSpace(InputText(payload["type"])),
				"status":     payload["status"],
				"payload":    string(rawPayload),
			},
		}).Result()
		if err == nil {
			if s.ttl > 0 {
				_ = s.client.Expire(ctx, streamKey, s.ttl).Err()
			}
			return id, nil
		}
		s.disableRedis()
	}
	return s.memory.Write(ctx, streamKey, payload)
}

func (s Service) Read(ctx context.Context, requestID string, lastID string, count int64, block time.Duration) ([]Entry, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("request_id 不能为空")
	}
	lastID = strings.TrimSpace(lastID)
	if lastID == "" {
		lastID = "0-0"
	}
	if count <= 0 {
		count = 100
	}

	if s.canUseRedis() {
		streams, err := s.client.XRead(ctx, &redis.XReadArgs{
			Streams: []string{s.Key(requestID), lastID},
			Count:   count,
			Block:   block,
		}).Result()
		if err == redis.Nil {
			return []Entry{}, nil
		}
		if err == nil {
			entries := make([]Entry, 0)
			for _, currentStream := range streams {
				for _, message := range currentStream.Messages {
					entries = append(entries, Entry{
						ID:      message.ID,
						Payload: DecodePayload(message.Values["payload"]),
					})
				}
			}
			return entries, nil
		}
		s.disableRedis()
	}

	return s.memory.Read(ctx, s.Key(requestID), lastID, count, block)
}

func (s Service) Key(requestID string) string {
	return StreamKey(s.namespace, requestID)
}

func StreamKey(namespace string, requestID string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = "default"
	}
	return namespace + ":stream:" + strings.TrimSpace(requestID)
}

func DecodePayload(value any) map[string]any {
	payload := map[string]any{}
	raw := strings.TrimSpace(fmt.Sprint(value))
	if raw == "" {
		return payload
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return map[string]any{"raw": raw}
	}
	return payload
}

func ResponsePayload(requestID string, responseType string, output map[string]any, msg string, status int) map[string]any {
	if output == nil {
		output = map[string]any{}
	}
	if strings.TrimSpace(responseType) == "" {
		responseType = "stream"
	}
	if status == 0 {
		status = 1
	}
	return map[string]any{
		"request_id": strings.TrimSpace(requestID),
		"type":       strings.TrimSpace(responseType),
		"output":     output,
		"msg":        strings.TrimSpace(msg),
		"status":     status,
	}
}

func ProgressPayload(requestID string, text string, progress int) map[string]any {
	output := map[string]any{
		"event": "progress",
		"text":  strings.TrimSpace(text),
	}
	if progress >= 0 {
		if progress > 100 {
			progress = 100
		}
		output["progress"] = progress
	}
	return ResponsePayload(requestID, "stream", output, "", 1)
}

func NextPayload(requestID string, lastID string, entries []Entry) map[string]any {
	if len(entries) == 0 {
		return ResponsePayloadWithStreamID(requestID, "stream", map[string]any{}, "", 1, lastID)
	}

	entry := entries[0]
	payload := entry.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payload["stream_id"] = entry.ID
	if strings.TrimSpace(InputText(payload["request_id"])) == "" {
		payload["request_id"] = requestID
	}
	if strings.TrimSpace(InputText(payload["type"])) == "" {
		payload["type"] = "stream"
	}
	if _, exists := payload["output"]; !exists {
		payload["output"] = map[string]any{}
	}
	if _, exists := payload["msg"]; !exists {
		payload["msg"] = ""
	}
	if _, exists := payload["status"]; !exists {
		payload["status"] = 1
	}
	return payload
}

func ResponsePayloadWithStreamID(requestID string, responseType string, output map[string]any, msg string, status int, streamID string) map[string]any {
	payload := ResponsePayload(requestID, responseType, output, msg, status)
	payload["stream_id"] = strings.TrimSpace(streamID)
	return payload
}

func InputText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(strings.Trim(fmt.Sprint(value), "\""))
}

func InputInt64(value any, fallback int64) int64 {
	valueText := strings.TrimSpace(InputText(value))
	if valueText == "" {
		return fallback
	}
	result, err := strconv.ParseInt(valueText, 10, 64)
	if err != nil {
		return fallback
	}
	return result
}

func (s Service) canUseRedis() bool {
	if s.client == nil {
		return false
	}
	if s.redisDisabled == nil {
		return true
	}
	return !s.redisDisabled.Load()
}

func (s Service) disableRedis() {
	if s.redisDisabled != nil {
		s.redisDisabled.Store(true)
	}
}

type memoryStreamStore struct {
	mu     sync.Mutex
	notify chan struct{}
	items  map[string][]memoryEntry
	seq    int64
	maxLen int64
	ttl    time.Duration
}

type memoryEntry struct {
	id        string
	payload   map[string]any
	createdAt time.Time
}

func newMemoryStreamStore(maxLen int64, ttl time.Duration) *memoryStreamStore {
	if maxLen <= 0 {
		maxLen = defaultMaxLen
	}
	return &memoryStreamStore{
		notify: make(chan struct{}),
		items:  map[string][]memoryEntry{},
		maxLen: maxLen,
		ttl:    ttl,
	}
}

func (s *memoryStreamStore) Write(ctx context.Context, key string, payload map[string]any) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.cleanupLocked(now)
	s.seq++
	id := fmt.Sprintf("%d-%d", now.UnixMilli(), s.seq)
	s.items[key] = append(s.items[key], memoryEntry{
		id:        id,
		payload:   clonePayload(payload),
		createdAt: now,
	})
	if int64(len(s.items[key])) > s.maxLen {
		s.items[key] = s.items[key][int64(len(s.items[key]))-s.maxLen:]
	}
	s.notifyWaitersLocked()
	return id, nil
}

func (s *memoryStreamStore) Read(ctx context.Context, key string, lastID string, count int64, block time.Duration) ([]Entry, error) {
	if count <= 0 {
		count = 100
	}
	deadline := time.Time{}
	if block > 0 {
		deadline = time.Now().Add(block)
	}

	for {
		s.mu.Lock()
		s.cleanupLocked(time.Now())
		entries := s.entriesAfterLocked(key, lastID, count)
		notify := s.notify
		s.mu.Unlock()

		if len(entries) > 0 || block <= 0 {
			return entries, nil
		}

		wait := time.Until(deadline)
		if wait <= 0 {
			return []Entry{}, nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-notify:
			timer.Stop()
		case <-timer.C:
			return []Entry{}, nil
		}
	}
}

func (s *memoryStreamStore) entriesAfterLocked(key string, lastID string, count int64) []Entry {
	rows := s.items[key]
	if len(rows) == 0 {
		return []Entry{}
	}
	result := make([]Entry, 0, minInt64(count, int64(len(rows))))
	for _, row := range rows {
		if compareStreamID(row.id, lastID) <= 0 {
			continue
		}
		result = append(result, Entry{
			ID:      row.id,
			Payload: clonePayload(row.payload),
		})
		if int64(len(result)) >= count {
			break
		}
	}
	return result
}

func (s *memoryStreamStore) cleanupLocked(now time.Time) {
	if s.ttl <= 0 {
		return
	}
	cutoff := now.Add(-s.ttl)
	for key, rows := range s.items {
		index := 0
		for index < len(rows) && rows[index].createdAt.Before(cutoff) {
			index++
		}
		if index >= len(rows) {
			delete(s.items, key)
			continue
		}
		if index > 0 {
			s.items[key] = rows[index:]
		}
	}
}

func (s *memoryStreamStore) notifyWaitersLocked() {
	close(s.notify)
	s.notify = make(chan struct{})
}

func clonePayload(payload map[string]any) map[string]any {
	next := make(map[string]any, len(payload))
	for key, value := range payload {
		next[key] = value
	}
	return next
}

func compareStreamID(left string, right string) int {
	leftTime, leftSeq := parseStreamID(left)
	rightTime, rightSeq := parseStreamID(right)
	if leftTime < rightTime {
		return -1
	}
	if leftTime > rightTime {
		return 1
	}
	if leftSeq < rightSeq {
		return -1
	}
	if leftSeq > rightSeq {
		return 1
	}
	return 0
}

func parseStreamID(value string) (int64, int64) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0
	}
	left, right, ok := strings.Cut(value, "-")
	if !ok {
		left = value
		right = "0"
	}
	timePart, _ := strconv.ParseInt(left, 10, 64)
	seqPart, _ := strconv.ParseInt(right, 10, 64)
	return timePart, seqPart
}

func minInt64(left int64, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func newRedisClientFromEnv(prefix string) *redis.Client {
	if url := strings.TrimSpace(os.Getenv(prefix + "_REDIS_URL")); url != "" {
		if opt, err := redis.ParseURL(url); err == nil {
			return redis.NewClient(opt)
		}
	}
	if url := strings.TrimSpace(os.Getenv("STREAM_REDIS_URL")); url != "" {
		if opt, err := redis.ParseURL(url); err == nil {
			return redis.NewClient(opt)
		}
	}
	if url := strings.TrimSpace(os.Getenv("ENERGON_REDIS_URL")); url != "" {
		if opt, err := redis.ParseURL(url); err == nil {
			return redis.NewClient(opt)
		}
	}
	if url := strings.TrimSpace(os.Getenv("REDIS_URL")); url != "" {
		if opt, err := redis.ParseURL(url); err == nil {
			return redis.NewClient(opt)
		}
	}
	return redis.NewClient(&redis.Options{
		Addr:     firstEnv("127.0.0.1:6379", prefix+"_REDIS_ADDR", "STREAM_REDIS_ADDR", "ENERGON_REDIS_ADDR", "REDIS_ADDR"),
		Password: firstEnv("", prefix+"_REDIS_PASSWORD", "STREAM_REDIS_PASSWORD", "ENERGON_REDIS_PASSWORD", "REDIS_PASSWORD"),
		DB:       int(envInt64(prefix+"_REDIS_DB", envInt64("STREAM_REDIS_DB", envInt64("ENERGON_REDIS_DB", envInt64("REDIS_DB", 0))))),
	})
}

func firstEnv(fallback string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return result
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

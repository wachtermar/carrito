package alcampo

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wachtermar/carrito/internal/httpx"
)

const (
	LiveSnapshotSchemaVersion = "1"
	LiveSnapshotManifestFile  = "snapshot_manifest.json"
	LiveSnapshotExitBlocked   = 31
	LiveSnapshotModeRecord    = "record"
	LiveSnapshotModeReplay    = "replay"
	LiveSnapshotModeNone      = ""
)

const (
	SnapshotKindSearch          = "search"
	SnapshotKindProductDetail   = "product_detail"
	SnapshotKindStore           = "store"
	SnapshotKindAvailability    = "availability"
	SnapshotKindUnknownReadonly = "unknown_readonly"
	SnapshotKindMutation        = "mutation"
)

type LiveSnapshotOptions struct {
	Mode       string
	Dir        string
	SnapshotID string
	StoreID    string
	StoreName  string
	BaseURL    string
	Strict     bool
}

type LiveSnapshotManifest struct {
	SchemaVersion    string                   `json:"schema_version"`
	SnapshotID       string                   `json:"snapshot_id"`
	CreatedAt        string                   `json:"created_at,omitempty"`
	StoreID          string                   `json:"store_id,omitempty"`
	StoreName        string                   `json:"store_name,omitempty"`
	BaseURL          string                   `json:"base_url,omitempty"`
	Mode             string                   `json:"mode"`
	Policy           LiveSnapshotPolicy       `json:"policy"`
	EntryCount       int                      `json:"entry_count"`
	Entries          []LiveSnapshotEntry      `json:"entries"`
	RedactionSummary SnapshotRedactionSummary `json:"redaction_summary"`
	ReplayStats      *LiveSnapshotReplayStats `json:"replay_stats,omitempty"`
	Warnings         []SnapshotIssue          `json:"warnings,omitempty"`
}

type LiveSnapshotPolicy struct {
	ReadOnlyOnly           bool  `json:"read_only_only"`
	StoreRawResponseBodies bool  `json:"store_raw_response_bodies"`
	StoreRequestHeaders    bool  `json:"store_request_headers"`
	StoreResponseHeaders   bool  `json:"store_response_headers"`
	MaxResponseBytes       int64 `json:"max_response_bytes,omitempty"`
	StrictReplay           bool  `json:"strict_replay"`
}

type LiveSnapshotEntry struct {
	Sequence            int                  `json:"sequence"`
	ID                  string               `json:"id"`
	RequestSignature    string               `json:"request_signature"`
	ShortSignature      string               `json:"short_signature"`
	RequestKind         string               `json:"request_kind"`
	Method              string               `json:"method"`
	URLPath             string               `json:"url_path,omitempty"`
	URLHostHash         string               `json:"url_host_hash,omitempty"`
	NormalizedQueryHash string               `json:"normalized_query_hash,omitempty"`
	NormalizedBodyHash  string               `json:"normalized_body_hash,omitempty"`
	DebugLabel          string               `json:"debug_label,omitempty"`
	StatusCode          int                  `json:"status_code"`
	ResponsePath        string               `json:"response_path"`
	ResponseSHA256      string               `json:"response_sha256"`
	ResponseBytes       int64                `json:"response_bytes"`
	ContentType         string               `json:"content_type,omitempty"`
	ExtractedShape      AlcampoSnapshotShape `json:"extracted_shape,omitempty"`
	RecordedAt          string               `json:"recorded_at,omitempty"`
	Warnings            []SnapshotIssue      `json:"warnings,omitempty"`
	HitCount            int                  `json:"hit_count,omitempty"`
}

type AlcampoSnapshotShape struct {
	ProductCount        int      `json:"product_count,omitempty"`
	HasProductID        bool     `json:"has_product_id,omitempty"`
	HasProductName      bool     `json:"has_product_name,omitempty"`
	HasPrice            bool     `json:"has_price,omitempty"`
	HasUnitPrice        bool     `json:"has_unit_price,omitempty"`
	HasPackageText      bool     `json:"has_package_text,omitempty"`
	HasImageURL         bool     `json:"has_image_url,omitempty"`
	HasOfferText        bool     `json:"has_offer_text,omitempty"`
	OfferExamples       []string `json:"offer_examples,omitempty"`
	HasNutritionFields  bool     `json:"has_nutrition_fields,omitempty"`
	NutritionFieldNames []string `json:"nutrition_field_names,omitempty"`
	ParserWarnings      []string `json:"parser_warnings,omitempty"`
}

type SnapshotRedactionSummary struct {
	CookiesRemoved       bool     `json:"cookies_removed"`
	AuthTokensRemoved    bool     `json:"auth_tokens_removed"`
	HeadersDropped       []string `json:"headers_dropped,omitempty"`
	QueryParamsDropped   []string `json:"query_params_dropped,omitempty"`
	BodyFieldsDropped    []string `json:"body_fields_dropped,omitempty"`
	SensitivePatternHits int      `json:"sensitive_pattern_hits,omitempty"`
}

type SnapshotIssue struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

type LiveSnapshotReplayStats struct {
	Hits   int `json:"hits"`
	Misses int `json:"misses"`
}

type LiveSnapshotSummary struct {
	Mode                 string `json:"mode,omitempty"`
	SnapshotID           string `json:"snapshot_id,omitempty"`
	SnapshotDir          string `json:"snapshot_dir,omitempty"`
	SnapshotManifestPath string `json:"snapshot_manifest_path,omitempty"`
	EntryCount           int    `json:"entry_count,omitempty"`
	ReplayStrict         bool   `json:"replay_strict,omitempty"`
	SnapshotSHA256       string `json:"snapshot_sha256,omitempty"`
	ReplayHits           int    `json:"replay_hits"`
	ReplayMisses         int    `json:"replay_misses"`
}

type SnapshotError struct {
	Code string
	Err  error
}

func (e SnapshotError) Error() string {
	if e.Err == nil {
		return "live snapshot error"
	}
	return e.Err.Error()
}

func (e SnapshotError) Unwrap() error {
	return e.Err
}

type liveSnapshotSession struct {
	mode              string
	dir               string
	responsesDir      string
	manifestPath      string
	manifest          LiveSnapshotManifest
	entriesBySig      map[string]*LiveSnapshotEntry
	responseBodyBySig map[string][]byte
	replayStats       LiveSnapshotReplayStats
	nextSequence      int
	finalized         bool
}

type snapshotResponseFile struct {
	SchemaVersion string `json:"schema_version"`
	StatusCode    int    `json:"status_code"`
	ContentType   string `json:"content_type,omitempty"`
	BodySHA256    string `json:"body_sha256"`
	BodyBase64    string `json:"body_base64"`
	RecordedAt    string `json:"recorded_at,omitempty"`
}

type snapshotRequestSignature struct {
	Method              string
	RequestKind         string
	URLPath             string
	URLHostHash         string
	NormalizedQueryHash string
	NormalizedBodyHash  string
	DebugLabel          string
	Signature           string
	ShortSignature      string
	DroppedQueryParams  []string
	DroppedBodyFields   []string
	SensitiveHits       int
}

func (c *Client) ConfigureLiveSnapshot(opts LiveSnapshotOptions) error {
	mode := strings.TrimSpace(strings.ToLower(opts.Mode))
	if mode == LiveSnapshotModeNone {
		c.snapshot = nil
		return nil
	}
	if strings.TrimSpace(opts.BaseURL) == "" {
		opts.BaseURL = c.BaseURL
	}
	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		return snapshotPolicyError("snapshot_dir_missing", "live snapshot directory is required")
	}
	switch mode {
	case LiveSnapshotModeRecord:
		session, err := newRecordSnapshotSession(opts)
		if err != nil {
			return err
		}
		c.snapshot = session
		return nil
	case LiveSnapshotModeReplay:
		session, err := newReplaySnapshotSession(opts)
		if err != nil {
			return err
		}
		if session.manifest.BaseURL != "" {
			c.BaseURL = session.manifest.BaseURL
		}
		c.snapshot = session
		return nil
	default:
		return snapshotPolicyError("snapshot_mode_invalid", "snapshot mode must be record or replay")
	}
}

func (c *Client) FinalizeLiveSnapshot() (*LiveSnapshotManifest, error) {
	if c.snapshot == nil {
		return nil, nil
	}
	manifest, err := c.snapshot.finalize()
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func (c *Client) LiveSnapshotSummary() *LiveSnapshotSummary {
	if c.snapshot == nil {
		return nil
	}
	return c.snapshot.summary()
}

func (c *Client) doSnapshot(req *http.Request) ([]byte, bool, error) {
	if c.snapshot == nil {
		return nil, false, nil
	}
	body, err := snapshotRequestBody(req)
	if err != nil {
		return nil, true, err
	}
	sig := buildSnapshotSignature(req, body, c.RegionID)
	if sig.RequestKind == SnapshotKindMutation {
		return nil, true, snapshotPolicyError("snapshot_mutation_blocked", "live snapshots refuse cart/auth/checkout/account mutation-like requests: "+req.URL.Path)
	}
	switch c.snapshot.mode {
	case LiveSnapshotModeReplay:
		data, statusCode, err := c.snapshot.replay(sig)
		if err == nil && (statusCode < 200 || statusCode > 399) {
			err = &httpx.StatusError{Method: req.Method, URL: req.URL.String(), StatusCode: statusCode, Status: fmt.Sprintf("%d snapshot replay", statusCode), Body: snippet(data)}
		}
		return data, true, err
	case LiveSnapshotModeRecord:
		return nil, false, nil
	default:
		return nil, false, nil
	}
}

func (c *Client) recordSnapshotResponse(req *http.Request, statusCode int, contentType string, body []byte) error {
	if c.snapshot == nil || c.snapshot.mode != LiveSnapshotModeRecord {
		return nil
	}
	reqBody, err := snapshotRequestBody(req)
	if err != nil {
		return err
	}
	sig := buildSnapshotSignature(req, reqBody, c.RegionID)
	if sig.RequestKind == SnapshotKindMutation {
		return snapshotPolicyError("snapshot_mutation_blocked", "live snapshots refuse cart/auth/checkout/account mutation-like requests: "+req.URL.Path)
	}
	return c.snapshot.record(sig, statusCode, contentType, body, c.BaseURL)
}

func newRecordSnapshotSession(opts LiveSnapshotOptions) (*liveSnapshotSession, error) {
	dir := strings.TrimSpace(opts.Dir)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	manifestPath := filepath.Join(dir, LiveSnapshotManifestFile)
	if _, err := os.Stat(manifestPath); err == nil {
		return nil, snapshotPolicyError("snapshot_manifest_exists", "snapshot directory already contains "+LiveSnapshotManifestFile)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	responsesDir := filepath.Join(dir, "responses")
	if err := os.MkdirAll(responsesDir, 0o700); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(opts.SnapshotID)
	if id == "" {
		id = "snapshot-" + strings.ReplaceAll(nowStamp(), ":", "-")
	}
	manifest := LiveSnapshotManifest{
		SchemaVersion: LiveSnapshotSchemaVersion,
		SnapshotID:    sanitizeSnapshotLabel(id, 80),
		CreatedAt:     nowStamp(),
		StoreID:       opts.StoreID,
		StoreName:     opts.StoreName,
		BaseURL:       strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/"),
		Mode:          LiveSnapshotModeRecord,
		Policy: LiveSnapshotPolicy{
			ReadOnlyOnly:           true,
			StoreRawResponseBodies: true,
			StoreRequestHeaders:    false,
			StoreResponseHeaders:   false,
			MaxResponseBytes:       10 << 20,
			StrictReplay:           false,
		},
		RedactionSummary: SnapshotRedactionSummary{
			CookiesRemoved:    true,
			AuthTokensRemoved: true,
			HeadersDropped:    []string{"Cookie", "Authorization", "Set-Cookie", "x-csrf-token", "customer-id", "visitor-id"},
		},
	}
	return &liveSnapshotSession{
		mode:              LiveSnapshotModeRecord,
		dir:               dir,
		responsesDir:      responsesDir,
		manifestPath:      manifestPath,
		manifest:          manifest,
		entriesBySig:      map[string]*LiveSnapshotEntry{},
		responseBodyBySig: map[string][]byte{},
	}, nil
}

func newReplaySnapshotSession(opts LiveSnapshotOptions) (*liveSnapshotSession, error) {
	dir := strings.TrimSpace(opts.Dir)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	manifestPath := filepath.Join(dir, LiveSnapshotManifestFile)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, snapshotPolicyError("snapshot_manifest_missing", "snapshot manifest is missing or unreadable: "+err.Error())
	}
	var manifest LiveSnapshotManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, snapshotPolicyError("snapshot_manifest_invalid", "snapshot manifest is invalid: "+err.Error())
	}
	if manifest.SchemaVersion != LiveSnapshotSchemaVersion {
		return nil, snapshotPolicyError("snapshot_manifest_schema", "snapshot manifest schema is unsupported")
	}
	entries := map[string]*LiveSnapshotEntry{}
	bodies := map[string][]byte{}
	for i := range manifest.Entries {
		entry := manifest.Entries[i]
		if existing, ok := entries[entry.RequestSignature]; ok && existing.ResponseSHA256 != entry.ResponseSHA256 {
			return nil, snapshotPolicyError("snapshot_duplicate_signature", "snapshot contains duplicate request signature with different responses")
		}
		body, status, contentType, err := readSnapshotResponseFile(filepath.Join(dir, entry.ResponsePath), entry.ResponseSHA256)
		if err != nil {
			return nil, err
		}
		entry.StatusCode = status
		entry.ContentType = contentType
		entries[entry.RequestSignature] = &entry
		bodies[entry.RequestSignature] = body
	}
	manifest.Mode = LiveSnapshotModeReplay
	manifest.Policy.StrictReplay = true
	return &liveSnapshotSession{
		mode:              LiveSnapshotModeReplay,
		dir:               dir,
		responsesDir:      filepath.Join(dir, "responses"),
		manifestPath:      manifestPath,
		manifest:          manifest,
		entriesBySig:      entries,
		responseBodyBySig: bodies,
	}, nil
}

func (s *liveSnapshotSession) record(sig snapshotRequestSignature, statusCode int, contentType string, body []byte, baseURL string) error {
	if int64(len(body)) > s.manifest.Policy.MaxResponseBytes {
		return snapshotPolicyError("snapshot_response_too_large", "response exceeds live snapshot max response size")
	}
	sum := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(sum[:])
	if existing, ok := s.entriesBySig[sig.Signature]; ok {
		if existing.ResponseSHA256 != bodyHash {
			return snapshotPolicyError("snapshot_duplicate_signature", "same request signature produced different response bodies")
		}
		existing.HitCount++
		return nil
	}
	s.nextSequence++
	short := sig.ShortSignature
	fileName := fmt.Sprintf("%06d_%s_%s.json", s.nextSequence, safeFilePart(sig.RequestKind), short)
	responsePath := filepath.Join("responses", fileName)
	responseFile := snapshotResponseFile{
		SchemaVersion: LiveSnapshotSchemaVersion,
		StatusCode:    statusCode,
		ContentType:   contentType,
		BodySHA256:    bodyHash,
		BodyBase64:    base64.StdEncoding.EncodeToString(body),
		RecordedAt:    nowStamp(),
	}
	data, err := json.MarshalIndent(responseFile, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.dir, responsePath), data, 0o600); err != nil {
		return err
	}
	entry := LiveSnapshotEntry{
		Sequence:            s.nextSequence,
		ID:                  fmt.Sprintf("%06d-%s", s.nextSequence, short),
		RequestSignature:    sig.Signature,
		ShortSignature:      short,
		RequestKind:         sig.RequestKind,
		Method:              sig.Method,
		URLPath:             sig.URLPath,
		URLHostHash:         sig.URLHostHash,
		NormalizedQueryHash: sig.NormalizedQueryHash,
		NormalizedBodyHash:  sig.NormalizedBodyHash,
		DebugLabel:          sig.DebugLabel,
		StatusCode:          statusCode,
		ResponsePath:        responsePath,
		ResponseSHA256:      bodyHash,
		ResponseBytes:       int64(len(body)),
		ContentType:         contentType,
		ExtractedShape:      extractSnapshotShape(body, contentType, baseURL),
		RecordedAt:          nowStamp(),
		HitCount:            1,
	}
	s.entriesBySig[sig.Signature] = &entry
	s.manifest.Entries = append(s.manifest.Entries, entry)
	s.responseBodyBySig[sig.Signature] = body
	s.manifest.RedactionSummary.QueryParamsDropped = appendUniqueStrings(s.manifest.RedactionSummary.QueryParamsDropped, sig.DroppedQueryParams...)
	s.manifest.RedactionSummary.BodyFieldsDropped = appendUniqueStrings(s.manifest.RedactionSummary.BodyFieldsDropped, sig.DroppedBodyFields...)
	s.manifest.RedactionSummary.SensitivePatternHits += sig.SensitiveHits
	return nil
}

func (s *liveSnapshotSession) replay(sig snapshotRequestSignature) ([]byte, int, error) {
	if sig.RequestKind == SnapshotKindMutation {
		return nil, 0, snapshotPolicyError("snapshot_mutation_blocked", "live snapshot replay refuses mutation-like requests")
	}
	entry, ok := s.entriesBySig[sig.Signature]
	if !ok {
		s.replayStats.Misses++
		return nil, 0, snapshotPolicyError("snapshot_replay_miss", "snapshot replay missing request signature for "+sig.DebugLabel)
	}
	body, ok := s.responseBodyBySig[sig.Signature]
	if !ok {
		return nil, 0, snapshotPolicyError("snapshot_response_missing", "snapshot response body is missing for "+entry.ID)
	}
	sum := sha256.Sum256(body)
	if got := hex.EncodeToString(sum[:]); got != entry.ResponseSHA256 {
		return nil, 0, snapshotPolicyError("snapshot_response_hash_mismatch", "snapshot response hash changed for "+entry.ID)
	}
	s.replayStats.Hits++
	return body, entry.StatusCode, nil
}

func (s *liveSnapshotSession) finalize() (*LiveSnapshotManifest, error) {
	if s.finalized {
		return &s.manifest, nil
	}
	if s.mode == LiveSnapshotModeRecord {
		s.manifest.EntryCount = len(s.manifest.Entries)
		sort.SliceStable(s.manifest.Entries, func(i, j int) bool {
			return s.manifest.Entries[i].Sequence < s.manifest.Entries[j].Sequence
		})
		data, err := json.MarshalIndent(s.manifest, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(s.manifestPath, data, 0o600); err != nil {
			return nil, err
		}
	} else if s.mode == LiveSnapshotModeReplay {
		stats := s.replayStats
		s.manifest.ReplayStats = &stats
		if s.manifest.Policy.StrictReplay && stats.Misses > 0 {
			return nil, snapshotPolicyError("snapshot_replay_miss", fmt.Sprintf("strict snapshot replay missed %d request signatures", stats.Misses))
		}
	}
	s.finalized = true
	return &s.manifest, nil
}

func (s *liveSnapshotSession) summary() *LiveSnapshotSummary {
	hash := ""
	if data, err := os.ReadFile(s.manifestPath); err == nil {
		sum := sha256.Sum256(data)
		hash = hex.EncodeToString(sum[:])
	}
	return &LiveSnapshotSummary{
		Mode:                 s.mode,
		SnapshotID:           s.manifest.SnapshotID,
		SnapshotDir:          s.dir,
		SnapshotManifestPath: s.manifestPath,
		EntryCount:           len(s.manifest.Entries),
		ReplayStrict:         s.manifest.Policy.StrictReplay || s.mode == LiveSnapshotModeReplay,
		SnapshotSHA256:       hash,
		ReplayHits:           s.replayStats.Hits,
		ReplayMisses:         s.replayStats.Misses,
	}
}

func buildSnapshotSignature(req *http.Request, body []byte, storeID string) snapshotRequestSignature {
	kind := classifySnapshotRequest(req)
	path := req.URL.EscapedPath()
	query, droppedQuery, debug := normalizedSnapshotQuery(req.URL.Query(), kind)
	bodyHash, droppedBody, bodyHits := normalizedSnapshotBodyHash(body)
	hostHash := shortHash(strings.ToLower(req.URL.Host))
	parts := []string{
		strings.ToUpper(req.Method),
		kind,
		path,
		query,
		bodyHash,
		"store=" + strings.TrimSpace(storeID),
	}
	raw := strings.Join(parts, "\n")
	sum := sha256.Sum256([]byte(raw))
	signature := hex.EncodeToString(sum[:])
	return snapshotRequestSignature{
		Method:              strings.ToUpper(req.Method),
		RequestKind:         kind,
		URLPath:             path,
		URLHostHash:         hostHash,
		NormalizedQueryHash: shortHash(query),
		NormalizedBodyHash:  bodyHash,
		DebugLabel:          sanitizeSnapshotLabel(debug, 120),
		Signature:           signature,
		ShortSignature:      signature[:12],
		DroppedQueryParams:  droppedQuery,
		DroppedBodyFields:   droppedBody,
		SensitiveHits:       bodyHits,
	}
}

func classifySnapshotRequest(req *http.Request) string {
	path := strings.ToLower(req.URL.EscapedPath())
	method := strings.ToUpper(req.Method)
	mutationMarkers := []string{"cart", "checkout", "payment", "login", "auth", "address", "order", "coupon", "customer", "account", "profile"}
	for _, marker := range mutationMarkers {
		if strings.Contains(path, marker) {
			return SnapshotKindMutation
		}
	}
	if method != http.MethodGet {
		return SnapshotKindMutation
	}
	switch {
	case path == "/" || path == "":
		return SnapshotKindStore
	case strings.Contains(path, "/product-pages/search"):
		return SnapshotKindSearch
	case strings.HasPrefix(path, "/products/"):
		return SnapshotKindProductDetail
	case strings.Contains(path, "/categories"):
		return SnapshotKindStore
	case strings.Contains(path, "/product-pages"):
		return SnapshotKindAvailability
	default:
		return SnapshotKindUnknownReadonly
	}
}

func normalizedSnapshotQuery(values url.Values, kind string) (string, []string, string) {
	dropNames := map[string]bool{
		"session": true, "sid": true, "token": true, "auth": true, "timestamp": true, "ts": true,
		"_": true, "cartid": true, "cart_id": true, "addressid": true, "address_id": true,
		"deliverydestinationid": true, "delivery_destination_id": true,
	}
	allowed := map[string][]string{}
	var dropped []string
	for key, vals := range values {
		norm := strings.ToLower(strings.TrimSpace(key))
		if dropNames[norm] || strings.Contains(norm, "token") || strings.Contains(norm, "session") || strings.Contains(norm, "cookie") {
			dropped = append(dropped, key)
			continue
		}
		copied := append([]string(nil), vals...)
		sort.Strings(copied)
		allowed[key] = copied
	}
	keys := sortedStringKeys(allowed)
	var parts []string
	for _, key := range keys {
		for _, val := range allowed[key] {
			parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(val))
		}
	}
	debug := kind
	if q := firstQueryValue(values, "q"); q != "" {
		debug = "search: " + q
	}
	return strings.Join(parts, "&"), dropped, debug
}

func normalizedSnapshotBodyHash(body []byte) (string, []string, int) {
	if len(body) == 0 {
		return "", nil, 0
	}
	var decoded any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err == nil {
		var dropped []string
		sanitized := redactSnapshotJSON(decoded, &dropped)
		data, _ := json.Marshal(sanitized)
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:]), dropped, countSensitiveHits(data)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil, countSensitiveHits(body)
}

func redactSnapshotJSON(value any, dropped *[]string) any {
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, val := range v {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "cookie") || strings.Contains(lower, "session") || strings.Contains(lower, "cart") || strings.Contains(lower, "address") || strings.Contains(lower, "auth") {
				*dropped = appendUniqueStrings(*dropped, key)
				continue
			}
			out[key] = redactSnapshotJSON(val, dropped)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = redactSnapshotJSON(v[i], dropped)
		}
		return out
	default:
		return value
	}
}

func snapshotRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func readSnapshotResponseFile(path, expectedHash string) ([]byte, int, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, "", snapshotPolicyError("snapshot_response_missing", "snapshot response file is missing: "+err.Error())
	}
	var file snapshotResponseFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, 0, "", snapshotPolicyError("snapshot_response_invalid", "snapshot response file is invalid: "+err.Error())
	}
	body, err := base64.StdEncoding.DecodeString(file.BodyBase64)
	if err != nil {
		return nil, 0, "", snapshotPolicyError("snapshot_response_invalid", "snapshot response body is not valid base64")
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	if hash != file.BodySHA256 || (expectedHash != "" && hash != expectedHash) {
		return nil, 0, "", snapshotPolicyError("snapshot_response_hash_mismatch", "snapshot response hash mismatch for "+filepath.Base(path))
	}
	return body, file.StatusCode, file.ContentType, nil
}

func extractSnapshotShape(body []byte, contentType, baseURL string) AlcampoSnapshotShape {
	var shape AlcampoSnapshotShape
	var root any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&root); err == nil {
		products := collectProducts(root, baseURL)
		applyProductsToSnapshotShape(&shape, products)
		shape.NutritionFieldNames = nutritionFieldNamesFromAny(root)
		shape.HasNutritionFields = len(shape.NutritionFieldNames) > 0
		return shape
	}
	if p, ok := productFromHTML(body, baseURL); ok {
		applyProductsToSnapshotShape(&shape, []Product{p})
	}
	if strings.Contains(strings.ToLower(string(body)), "nutrition") {
		shape.HasNutritionFields = true
		shape.NutritionFieldNames = []string{"nutrition"}
	}
	return shape
}

func applyProductsToSnapshotShape(shape *AlcampoSnapshotShape, products []Product) {
	shape.ProductCount = len(products)
	offerSet := map[string]bool{}
	for _, p := range products {
		if p.ID != "" || p.SKU != "" {
			shape.HasProductID = true
		}
		if p.Name != "" {
			shape.HasProductName = true
		}
		if p.Price.Amount != "" || p.Price.Cents != 0 {
			shape.HasPrice = true
		}
		if p.UnitPrice.Amount != "" || p.UnitPrice.Cents != 0 || p.Unit != "" {
			shape.HasUnitPrice = true
		}
		if p.Size != "" {
			shape.HasPackageText = true
		}
		if len(p.Images) > 0 {
			shape.HasImageURL = true
		}
		if p.Nutrition != "" {
			shape.HasNutritionFields = true
			shape.NutritionFieldNames = appendUniqueStrings(shape.NutritionFieldNames, "nutrition")
		}
		for _, offer := range p.Offers {
			text := strings.TrimSpace(firstNonEmpty(offer.Name, offer.Description, offer.Type))
			if text != "" {
				shape.HasOfferText = true
				if len(shape.OfferExamples) < 3 && !offerSet[text] {
					offerSet[text] = true
					shape.OfferExamples = append(shape.OfferExamples, text)
				}
			}
		}
	}
	sort.Strings(shape.NutritionFieldNames)
}

func nutritionFieldNamesFromAny(value any) []string {
	fields := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for key, val := range x {
				lower := strings.ToLower(key)
				if strings.Contains(lower, "nutrition") || strings.Contains(lower, "nutrient") || strings.Contains(lower, "calorie") || strings.Contains(lower, "kcal") || strings.Contains(lower, "protein") || strings.Contains(lower, "fat") || strings.Contains(lower, "salt") {
					fields[key] = true
				}
				walk(val)
			}
		case []any:
			for _, val := range x {
				walk(val)
			}
		}
	}
	walk(value)
	return sortedBoolKeys(fields)
}

func snapshotPolicyError(code, message string) error {
	return SnapshotError{Code: code, Err: errors.New(message)}
}

func firstQueryValue(values url.Values, key string) string {
	for actual, vals := range values {
		if strings.EqualFold(actual, key) && len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

func shortHash(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func safeFilePart(value string) string {
	value = strings.ToLower(sanitizeSnapshotLabel(value, 40))
	value = strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-").Replace(value)
	if value == "" {
		return "request"
	}
	return value
}

func sanitizeSnapshotLabel(value string, max int) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '.' || r == ':':
			b.WriteRune(r)
		}
		if max > 0 && b.Len() >= max {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func countSensitiveHits(data []byte) int {
	lower := strings.ToLower(string(data))
	hits := 0
	for _, marker := range []string{"authorization", "bearer ", "set-cookie", "x-csrf-token", "access_token", "refresh_token"} {
		hits += strings.Count(lower, marker)
	}
	return hits
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := map[string]bool{}
	out := values[:0]
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedBoolKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		if m[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func nowStamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseSnapshotSequence(path string) int {
	base := filepath.Base(path)
	raw := strings.SplitN(base, "_", 2)[0]
	n, _ := strconv.Atoi(raw)
	return n
}

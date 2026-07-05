package auth

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

type Session struct {
	Cookie                string `json:"-"`
	BearerToken           string `json:"-"`
	CSRFToken             string `json:"-"`
	CustomerID            string `json:"-"`
	VisitorID             string `json:"-"`
	SourceVersion         string `json:"source_version,omitempty"`
	RegionID              string `json:"region_id,omitempty"`
	RetailerRegionID      string `json:"retailer_region_id,omitempty"`
	RegionName            string `json:"region_name,omitempty"`
	DeliveryDestinationID string `json:"delivery_destination_id,omitempty"`
	ImportedAt            string `json:"imported_at,omitempty"`
}

func ParseText(data []byte) Session {
	var s Session
	extractLocationHints(&s, string(data))
	s.ImportedAt = time.Now().UTC().Format(time.RFC3339)
	return s
}

func ParseCurl(data []byte) (Session, error) {
	text := string(data)
	var s Session
	for _, h := range parseCurlHeaders(text) {
		applyHeader(&s, h.Name, h.Value)
	}
	extractLocationHints(&s, text)
	s.ImportedAt = time.Now().UTC().Format(time.RFC3339)
	return s, nil
}

func ParseHAR(data []byte) (Session, error) {
	var har struct {
		Log struct {
			Entries []struct {
				Request struct {
					URL         string `json:"url"`
					Headers     []hdr  `json:"headers"`
					QueryString []hdr  `json:"queryString"`
					PostData    struct {
						Text string `json:"text"`
					} `json:"postData"`
				} `json:"request"`
				Response struct {
					Content struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"response"`
			} `json:"entries"`
		} `json:"log"`
	}
	if err := json.Unmarshal(data, &har); err != nil {
		return Session{}, err
	}
	var s Session
	for _, entry := range har.Log.Entries {
		for _, h := range entry.Request.Headers {
			applyHeader(&s, h.Name, h.Value)
		}
		for _, q := range entry.Request.QueryString {
			applyLocationKey(&s, q.Name, q.Value)
		}
		extractLocationHints(&s, entry.Request.URL)
		extractLocationHints(&s, entry.Request.PostData.Text)
		extractLocationHints(&s, entry.Response.Content.Text)
	}
	s.ImportedAt = time.Now().UTC().Format(time.RFC3339)
	return s, nil
}

type hdr struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

var curlHeaderRE = regexp.MustCompile(`(?is)(?:-H|--header)\s+(?:'([^']*)'|"([^"]*)")`)

func parseCurlHeaders(text string) []hdr {
	var out []hdr
	for _, m := range curlHeaderRE.FindAllStringSubmatch(text, -1) {
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		name, value, ok := strings.Cut(raw, ":")
		if !ok {
			continue
		}
		out = append(out, hdr{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)})
	}
	return out
}

func applyHeader(s *Session, name, value string) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "cookie":
		if value != "" {
			s.Cookie = value
		}
	case "authorization":
		if token, ok := strings.CutPrefix(strings.TrimSpace(value), "Bearer "); ok {
			s.BearerToken = strings.TrimSpace(token)
		}
	case "x-csrf-token", "x-xsrf-token", "csrf-token":
		if value != "" {
			s.CSRFToken = value
		}
	case "customer-id", "customerid":
		if value != "" {
			s.CustomerID = value
		}
	case "visitor-id", "visitorid":
		if value != "" {
			s.VisitorID = value
		}
	}
}

var uuidRE = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
var regionParamRE = regexp.MustCompile(`(?i)(regionId|resolvedRegionId|retailerRegionId|retailer_region_id|regionName|region_name|deliveryDestinationId|delivery_destination_id|customerId|customer_id|visitorId|visitor_id)=([^&\s'"]+)`)
var jsonHintRE = regexp.MustCompile(`(?i)["'](regionId|resolvedRegionId|retailerRegionId|retailer_region_id|regionName|region_name|deliveryDestinationId|delivery_destination_id|customerId|customer_id|visitorId|visitor_id)["']\s*:\s*["']?([^,"'\s}]+)`)

func extractLocationHints(s *Session, text string) {
	for _, m := range regionParamRE.FindAllStringSubmatch(text, -1) {
		applyLocationKey(s, m[1], m[2])
	}
	for _, m := range jsonHintRE.FindAllStringSubmatch(text, -1) {
		applyLocationKey(s, m[1], m[2])
	}
	if s.RegionID == "" {
		if id := uuidRE.FindString(text); id != "" {
			s.RegionID = id
		}
	}
}

func applyLocationKey(s *Session, key, value string) {
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "null") || strings.EqualFold(value, "undefined") {
		return
	}
	switch key {
	case "regionid", "region_id", "resolvedregionid":
		s.RegionID = value
	case "retailerregionid", "retailer_region_id":
		s.RetailerRegionID = value
	case "regionname", "region_name":
		s.RegionName = value
	case "deliverydestinationid", "delivery_destination_id":
		s.DeliveryDestinationID = value
	case "customerid", "customer_id":
		s.CustomerID = value
	case "visitorid", "visitor_id":
		s.VisitorID = value
	}
}

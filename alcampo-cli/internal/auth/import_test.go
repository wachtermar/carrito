package auth

import "testing"

func TestParseCurl(t *testing.T) {
	curl := `curl 'https://www.compraonline.alcampo.es/api/x?regionId=ac90d761-9d58-4918-a37d-dd14e1ce384a' \
  -H 'cookie: a=b; c=d' \
  -H 'authorization: Bearer token-123' \
  -H 'x-csrf-token: csrf-456' \
  -H 'customer-id: customer-789' \
  -H 'visitor-id: visitor-012'`
	s, err := ParseCurl([]byte(curl))
	if err != nil {
		t.Fatal(err)
	}
	if s.Cookie != "a=b; c=d" || s.BearerToken != "token-123" || s.CSRFToken != "csrf-456" {
		t.Fatalf("unexpected session: %+v", s)
	}
	if s.CustomerID != "customer-789" || s.VisitorID != "visitor-012" {
		t.Fatalf("ids customer=%q visitor=%q", s.CustomerID, s.VisitorID)
	}
	if s.RegionID != "ac90d761-9d58-4918-a37d-dd14e1ce384a" {
		t.Fatalf("region id = %q", s.RegionID)
	}
}

func TestParseHAR(t *testing.T) {
	har := `{"log":{"entries":[{"request":{"url":"https://x.test/api?retailerRegionId=5","headers":[{"name":"Cookie","value":"sid=1"}],"queryString":[{"name":"regionName","value":"Vaguada"}],"postData":{"text":"{\"deliveryDestinationId\":\"dest-1\"}"}}}]}}`
	s, err := ParseHAR([]byte(har))
	if err != nil {
		t.Fatal(err)
	}
	if s.Cookie != "sid=1" || s.RetailerRegionID != "5" || s.RegionName != "Vaguada" || s.DeliveryDestinationID != "dest-1" {
		t.Fatalf("unexpected session: %+v", s)
	}
}

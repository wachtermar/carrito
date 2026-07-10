package alcampo

import (
	"context"
	"net/url"
)

type DeliveryDestination struct {
	DeliveryDestinationID string        `json:"delivery_destination_id,omitempty"`
	AddressID             string        `json:"address_id,omitempty"`
	Name                  string        `json:"name,omitempty"`
	FormattedAddress      string        `json:"formatted_address,omitempty"`
	PostalCode            string        `json:"postal_code,omitempty"`
	CountryCode           string        `json:"country_code,omitempty"`
	DeliveryMethod        string        `json:"delivery_method,omitempty"`
	DeliveryType          string        `json:"delivery_type,omitempty"`
	Deliverability        string        `json:"deliverability,omitempty"`
	ResolvedRegionID      string        `json:"resolved_region_id,omitempty"`
	IsPrimary             bool          `json:"is_primary,omitempty"`
	Coordinates           Coordinates   `json:"coordinates,omitempty"`
	Propositions          []Proposition `json:"propositions,omitempty"`
}

type Coordinates struct {
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
}

type Proposition struct {
	DeliveryPropositionID string `json:"delivery_proposition_id,omitempty"`
	PropositionType       string `json:"proposition_type,omitempty"`
	RegionID              string `json:"region_id,omitempty"`
}

func (c *Client) DeliveryAddresses(ctx context.Context) ([]DeliveryDestination, error) {
	q := url.Values{}
	q.Set("deliveryMethod", "HOME_DELIVERY")
	var root any
	if err := c.getJSON(ctx, "/api/ecomdeliverydestinations/v4/delivery-addresses", q, c.BaseURL+"/delivery", "delivery", &root); err != nil {
		return nil, err
	}
	return collectDeliveryDestinations(root), nil
}

func (c *Client) DeliveryAddress(ctx context.Context, id string) (DeliveryDestination, error) {
	var root any
	if err := c.getJSON(ctx, "/api/ecomdeliverydestinations/v4/delivery-addresses/"+url.PathEscape(id), nil, c.BaseURL+"/delivery", "delivery", &root); err != nil {
		return DeliveryDestination{}, err
	}
	destinations := collectDeliveryDestinations(root)
	if len(destinations) == 0 {
		return DeliveryDestination{}, ErrNotFound
	}
	return destinations[0], nil
}

func collectDeliveryDestinations(root any) []DeliveryDestination {
	var out []DeliveryDestination
	walk(root, func(m map[string]any) {
		id := stringFromKeys(m, "deliveryDestinationId", "delivery_destination_id")
		if id == "" {
			return
		}
		d := DeliveryDestination{
			DeliveryDestinationID: id,
			AddressID:             stringFromKeys(m, "addressId", "address_id"),
			Name:                  stringFromKeys(m, "name"),
			FormattedAddress:      stringFromKeys(m, "formattedAddress", "formatted_address"),
			PostalCode:            stringFromKeys(m, "postalCode", "postal_code"),
			CountryCode:           stringFromKeys(m, "countryCode", "country_code"),
			DeliveryMethod:        stringFromKeys(m, "deliveryMethod", "delivery_method"),
			DeliveryType:          stringFromKeys(m, "deliveryType", "delivery_type"),
			Deliverability:        stringFromKeys(m, "deliverability"),
			ResolvedRegionID:      stringFromKeys(m, "resolvedRegionId", "resolved_region_id", "regionId"),
			Propositions:          propositionsFrom(mapValue(m, "propositions")),
		}
		if v := boolFromKeys(m, "isPrimary", "primary"); v != nil {
			d.IsPrimary = *v
		}
		if coords, ok := mapValue(m, "coordinates").(map[string]any); ok {
			d.Coordinates.Latitude = floatFrom(coords["latitude"])
			d.Coordinates.Longitude = floatFrom(coords["longitude"])
		}
		if d.ResolvedRegionID == "" && len(d.Propositions) > 0 {
			d.ResolvedRegionID = d.Propositions[0].RegionID
		}
		out = append(out, d)
	})
	return out
}

func propositionsFrom(v any) []Proposition {
	var out []Proposition
	items, ok := v.([]any)
	if !ok {
		return out
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, Proposition{
			DeliveryPropositionID: stringFromKeys(m, "deliveryPropositionId", "delivery_proposition_id"),
			PropositionType:       stringFromKeys(m, "propositionType", "proposition_type"),
			RegionID:              stringFromKeys(m, "regionId", "region_id"),
		})
	}
	return out
}

func firstString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

package alcampo

import (
	"context"
	"net/url"

	"github.com/wachtermar/carrito/internal/money"
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

type SlotsRequest struct {
	DeliveryDestinationID string `json:"deliveryDestinationId"`
	RegionID              string `json:"regionId"`
	DisplayConfiguration  string `json:"displayConfiguration"`
	ShippingGroupType     string `json:"shippingGroupType"`
	NumberOfDays          int    `json:"numberOfDays"`
}

func (c *Client) DeliverySlots(ctx context.Context, deliveryDestinationID, regionID string, days int) (any, error) {
	if days <= 0 {
		days = 7
	}
	body := SlotsRequest{
		DeliveryDestinationID: deliveryDestinationID,
		RegionID:              regionID,
		DisplayConfiguration:  "DELIVERY_METHOD",
		ShippingGroupType:     "Alcampo Vans",
		NumberOfDays:          days,
	}
	var root any
	if err := c.postJSON(ctx, "/api/ecomslots/v2/slots", body, c.BaseURL+"/delivery", "delivery", &root); err != nil {
		return nil, err
	}
	return root, nil
}

type DeliverySlot struct {
	SlotID       string      `json:"slot_id,omitempty"`
	Day          string      `json:"day,omitempty"`
	StartTime    string      `json:"start_time,omitempty"`
	EndTime      string      `json:"end_time,omitempty"`
	TimeZoneID   string      `json:"time_zone_id,omitempty"`
	DeliveryType string      `json:"delivery_type,omitempty"`
	CarrierName  string      `json:"carrier_name,omitempty"`
	Price        money.Money `json:"price,omitempty"`
	MinimumOrder money.Money `json:"minimum_order,omitempty"`
	EcoSlot      bool        `json:"eco_slot,omitempty"`
}

func CollectDeliverySlots(root any) []DeliverySlot {
	var out []DeliverySlot
	seen := map[string]bool{}
	var walkWithDay func(any, string)
	walkWithDay = func(v any, day string) {
		switch t := v.(type) {
		case map[string]any:
			if d := stringFromKeys(t, "day", "date"); d != "" {
				day = d
			}
			if id := stringFromKeys(t, "slotId", "slot_id", "id"); id != "" {
				if !seen[id] {
					seen[id] = true
					out = append(out, deliverySlotFromMap(t, day))
				}
			}
			for _, child := range t {
				walkWithDay(child, day)
			}
		case []any:
			for _, child := range t {
				walkWithDay(child, day)
			}
		}
	}
	walkWithDay(root, "")
	return out
}

func deliverySlotFromMap(m map[string]any, day string) DeliverySlot {
	window := mapFromKeys(m, "slotWindow", "window", "timeWindow")
	slot := DeliverySlot{
		SlotID:       stringFromKeys(m, "slotId", "slot_id", "id"),
		Day:          day,
		StartTime:    stringFromKeys(m, "startTime", "start"),
		EndTime:      stringFromKeys(m, "endTime", "end"),
		TimeZoneID:   stringFromKeys(m, "timeZoneId", "time_zone_id"),
		DeliveryType: stringFromKeys(m, "deliveryType", "delivery_type"),
		CarrierName:  stringFromKeys(m, "carrierName", "carrier_name"),
	}
	if window != nil {
		slot.StartTime = firstString(slot.StartTime, stringFromKeys(window, "startTime", "start"))
		slot.EndTime = firstString(slot.EndTime, stringFromKeys(window, "endTime", "end"))
		slot.TimeZoneID = firstString(slot.TimeZoneID, stringFromKeys(window, "timeZoneId", "time_zone_id"))
	}
	slot.Price = priceFrom(mapValue(m, "deliveryPrice", "price"))
	slot.MinimumOrder = priceFrom(mapValue(m, "minimumOrder", "minimumCheckoutThreshold", "minimumOrderValue"))
	if v := boolFromKeys(m, "ecoSlot", "eco"); v != nil {
		slot.EcoSlot = *v
	}
	return slot
}

func FindDeliverySlot(root any, slotID string) (DeliverySlot, bool) {
	for _, slot := range CollectDeliverySlots(root) {
		if slot.SlotID == slotID {
			return slot, true
		}
	}
	return DeliverySlot{}, false
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

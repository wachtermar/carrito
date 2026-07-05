package alcampo

import (
	"context"
	"errors"
)

var ErrCheckoutUnsupported = errors.New("unknown or unsupported checkout command; payment and order submission are disabled")

type CheckoutStartRequest struct {
	ShippingGroupType string `json:"shippingGroupType"`
}

type SlotReservationRequest struct {
	RegionID              string `json:"regionId"`
	SlotID                string `json:"slotId"`
	DeliveryDestinationID string `json:"deliveryDestinationId"`
	ExternalAddress       any    `json:"externalAddress,omitempty"`
}

func (c *Client) CheckoutStart(ctx context.Context, shippingGroupType string) (any, error) {
	if shippingGroupType == "" {
		shippingGroupType = "Alcampo Vans"
	}
	var root any
	if err := c.postJSON(ctx, "/api/cart/v1/carts/active/checkout-start", CheckoutStartRequest{ShippingGroupType: shippingGroupType}, c.BaseURL+"/basket", "basket", &root); err != nil {
		return nil, err
	}
	return root, nil
}

func (c *Client) ReserveSlot(ctx context.Context, req SlotReservationRequest) (any, error) {
	var root any
	if err := c.postJSON(ctx, "/api/ecomslots/v1/slots/reservation", req, c.BaseURL+"/delivery", "delivery", &root); err != nil {
		return nil, err
	}
	return root, nil
}

func (c *Client) ConfirmSlot(ctx context.Context, body any) (any, error) {
	var root any
	if err := c.putJSON(ctx, "/api/ecomslots/v1/slots/confirm", body, c.BaseURL+"/delivery", "delivery", &root); err != nil {
		return nil, err
	}
	return root, nil
}

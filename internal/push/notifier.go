package push

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Message types the rider app dispatches on. They are part of the contract with
// the app (fcm background handler); change them together.
const (
	TypeOfferCreated = "DELIVERY_ORDER_REQUEST"
	TypeOfferClosed  = "DELIVERY_ORDER_REQUEST_CLOSED"

	// APNSCategoryOffer is the iOS notification category that carries the
	// Accept and Decline actions, once the iOS app registers it.
	APNSCategoryOffer = "DELIVERY_OFFER"

	// ReasonAssignedToOther is why an offer closed for a rider: another rider
	// accepted first.
	ReasonAssignedToOther = "assigned_to_other"
)

// DeviceToken is one push registration of a rider's device.
type DeviceToken struct {
	Token    string
	Platform string
}

// TokenStore is where a rider's device tokens live.
type TokenStore interface {
	DeviceTokens(ctx context.Context, riderID string) ([]DeviceToken, error)
	RemoveDeviceToken(ctx context.Context, riderID, token string) error
}

// Offer is what a push may say about a delivery offer.
//
// It deliberately has no customer name, phone or drop address. Those are shown
// only after the rider accepts, and a notification can appear on a lock screen
// before anyone has accepted anything.
type Offer struct {
	RequestID int
	OrderID   int
	// OrderType is "food" or "grocery".
	OrderType      string
	RestaurantName string
	PickupAddress  string
	// DeliveryArea is a coarse, privacy-safe description of where the order
	// goes. See DeliveryAreaLabel.
	DeliveryArea string
	// DistanceKm is from the rider to the pickup.
	DistanceKm float64
	// DeliveryDistanceKm is the rounded straight line from pickup to drop.
	DeliveryDistanceKm float64
	// Amount is the order value the offer already carried. It is not the
	// rider's payout, which is credited on delivery and is not part of an offer.
	Amount      float64
	PaymentMode string
	ExpiresAt   time.Time
}

// OfferClosure tells a rider's devices that an offer they may be looking at is
// gone.
type OfferClosure struct {
	OrderID   int
	OrderType string
	Reason    string
}

// DeliveryAreaLabel is the delivery area shown in a notification.
//
// The data model has no structured locality for a delivery, and the customer's
// address is withheld until acceptance (see BuildDeliveryOrderRequestPayload),
// so the area is the rounded distance the app already shows on the offer sheet.
// If a locality field is added later, this is the only place that needs to
// change; a heuristic over the free-text address is not used because a wrong
// guess could put a house number on a lock screen.
func DeliveryAreaLabel(deliveryDistanceKm float64) string {
	if deliveryDistanceKm <= 0 {
		return ""
	}
	return fmt.Sprintf("Approx. %.0f km from pickup", deliveryDistanceKm)
}

// Notifier turns dispatch events into pushes. Every method returns at once:
// dispatch must never wait for Google, and a failed push never fails dispatch.
type Notifier struct {
	sender  Sender
	tokens  TokenStore
	now     func() time.Time
	timeout time.Duration

	sem chan struct{}
	wg  sync.WaitGroup
}

// NewNotifier returns a notifier. sender and tokens are required.
func NewNotifier(sender Sender, tokens TokenStore) *Notifier {
	return &Notifier{
		sender:  sender,
		tokens:  tokens,
		now:     time.Now,
		timeout: 15 * time.Second,
		// A burst of offers across many riders is bounded; extra pushes are
		// dropped (the rider still has the socket and the polling fallback)
		// rather than queueing behind a slow Google endpoint.
		sem: make(chan struct{}, 64),
	}
}

// Wait blocks until every push started so far has finished. Tests use it;
// production never does.
func (n *Notifier) Wait() { n.wg.Wait() }

// OfferCreated pushes a new offer to every device the rider has registered.
func (n *Notifier) OfferCreated(riderID string, offer Offer) {
	if n == nil {
		return
	}
	remaining := offer.ExpiresAt.Sub(n.now())
	if remaining <= 0 {
		// Already expired: nothing to act on.
		return
	}
	n.dispatch(riderID, "offer", func(token DeviceToken) Message {
		return buildOfferMessage(token, offer, remaining)
	})
}

// OfferClosed tells the rider's devices to drop the offer for closure.OrderID.
func (n *Notifier) OfferClosed(riderID string, closure OfferClosure) {
	n.dispatch(riderID, "offer_closed", func(token DeviceToken) Message {
		return buildClosureMessage(token, closure)
	})
}

func (n *Notifier) dispatch(riderID, kind string, build func(DeviceToken) Message) {
	if n == nil || n.sender == nil || n.tokens == nil || strings.TrimSpace(riderID) == "" {
		return
	}
	select {
	case n.sem <- struct{}{}:
	default:
		log.Printf("[PUSH] dropped %s push rider_id=%s reason=backlog", kind, riderID)
		return
	}
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		defer func() { <-n.sem }()
		defer func() {
			// A push is best effort and runs off the dispatch path.
			if recovered := recover(); recovered != nil {
				log.Printf("[PUSH] recovered panic kind=%s rider_id=%s: %v", kind, riderID, recovered)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
		defer cancel()

		tokens, err := n.tokens.DeviceTokens(ctx, riderID)
		if err != nil {
			log.Printf("[PUSH] token lookup failed kind=%s rider_id=%s err=%v", kind, riderID, err)
			return
		}
		if len(tokens) == 0 {
			// Not an error: a rider who never enabled notifications is
			// reachable over the socket and polling only.
			log.Printf("[PUSH] no device token kind=%s rider_id=%s", kind, riderID)
			return
		}
		sent, removed, failed := 0, 0, 0
		for _, token := range tokens {
			err := n.sender.Send(ctx, build(token))
			switch {
			case err == nil:
				sent++
			case errors.Is(err, ErrUnregistered):
				removed++
				if rmErr := n.tokens.RemoveDeviceToken(ctx, riderID, token.Token); rmErr != nil {
					log.Printf("[PUSH] could not forget retired token rider_id=%s err=%v", riderID, rmErr)
				}
			default:
				failed++
				log.Printf("[PUSH] send failed kind=%s rider_id=%s err=%v", kind, riderID, err)
			}
		}
		log.Printf("[PUSH] kind=%s rider_id=%s devices=%d sent=%d retired=%d failed=%d",
			kind, riderID, len(tokens), sent, removed, failed)
	}()
}

func buildOfferMessage(token DeviceToken, offer Offer, remaining time.Duration) Message {
	orderType := offer.OrderType
	if orderType == "" {
		orderType = "food"
	}
	ref := "#" + strconv.Itoa(offer.OrderID)
	data := map[string]string{
		"type":                 TypeOfferCreated,
		"request_id":           strconv.Itoa(offer.RequestID),
		"order_id":             strconv.Itoa(offer.OrderID),
		"order_ref":            ref,
		"order_type":           orderType,
		"restaurant_name":      offer.RestaurantName,
		"pickup_address":       offer.PickupAddress,
		"delivery_area":        offer.DeliveryArea,
		"distance_km":          strconv.FormatFloat(offer.DistanceKm, 'f', 2, 64),
		"delivery_distance_km": strconv.FormatFloat(offer.DeliveryDistanceKm, 'f', 0, 64),
		"amount":               strconv.FormatFloat(offer.Amount, 'f', 2, 64),
		"payment_mode":         offer.PaymentMode,
		"expires_at":           offer.ExpiresAt.UTC().Format(time.RFC3339),
	}
	msg := Message{
		Token:       token.Token,
		Data:        data,
		TTL:         remaining,
		CollapseKey: "offer-" + strconv.Itoa(offer.RequestID),
	}
	if strings.EqualFold(token.Platform, "ios") {
		msg.Alert = &Alert{Title: "New delivery request", Body: offerSummary(ref, offer)}
		msg.APNSCategory = APNSCategoryOffer
	}
	return msg
}

func buildClosureMessage(token DeviceToken, closure OfferClosure) Message {
	orderType := closure.OrderType
	if orderType == "" {
		orderType = "food"
	}
	reason := closure.Reason
	if reason == "" {
		reason = ReasonAssignedToOther
	}
	return Message{
		Token: token.Token,
		Data: map[string]string{
			"type":       TypeOfferClosed,
			"order_id":   strconv.Itoa(closure.OrderID),
			"order_type": orderType,
			"reason":     reason,
		},
		// Late delivery of a closure is pointless: by then the offer's own
		// expiry has cleared the notification.
		TTL:         time.Minute,
		CollapseKey: "offer-closed-" + strconv.Itoa(closure.OrderID),
	}
}

// offerSummary is the one-line text of an iOS alert: enough to decide, nothing
// that identifies the customer.
func offerSummary(ref string, offer Offer) string {
	parts := []string{ref}
	if name := strings.TrimSpace(offer.RestaurantName); name != "" {
		parts = append(parts, name)
	}
	if offer.DistanceKm > 0 {
		parts = append(parts, fmt.Sprintf("%.1f km to pickup", offer.DistanceKm))
	}
	if area := strings.TrimSpace(offer.DeliveryArea); area != "" {
		parts = append(parts, area)
	}
	return strings.Join(parts, " · ")
}

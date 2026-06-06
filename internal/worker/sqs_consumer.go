package worker

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

// SQSConsumer polls the SQS queue for ORDER_PLACED events.
type SQSConsumer struct {
	client      *sqs.Client
	queueURL    string
	deliverySvc *service.DeliveryService
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// NewSQSConsumer creates a new consumer.
func NewSQSConsumer(region, queueURL string, deliverySvc *service.DeliveryService) (*SQSConsumer, error) {
	if queueURL == "" {
		return nil, nil // Queue not configured, return nil
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, err
	}

	client := sqs.NewFromConfig(cfg)

	return &SQSConsumer{
		client:      client,
		queueURL:    queueURL,
		deliverySvc: deliverySvc,
		stopChan:    make(chan struct{}),
	}, nil
}

// Start begins polling the SQS queue.
func (c *SQSConsumer) Start() {
	if c == nil {
		return
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		log.Printf("[SQS] Starting consumer for queue: %s", c.queueURL)

		for {
			select {
			case <-c.stopChan:
				log.Println("[SQS] Stopping consumer...")
				return
			default:
				c.poll()
			}
		}
	}()
}

// Stop gracefully stops the consumer.
func (c *SQSConsumer) Stop() {
	if c == nil {
		return
	}
	close(c.stopChan)
	c.wg.Wait()
	log.Println("[SQS] Consumer stopped")
}

func (c *SQSConsumer) poll() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(c.queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     10, // Long polling
	})

	if err != nil {
		// Log but don't crash, SQS might be temporarily down
		log.Printf("[SQS] Error receiving messages: %v", err)
		time.Sleep(5 * time.Second) // backoff
		return
	}

	for _, msg := range result.Messages {
		if err := c.processMessage(msg); err != nil {
			log.Printf("[SQS] Failed to process message %s: %v", *msg.MessageId, err)
			// Message is NOT deleted, it will become visible again after VisibilityTimeout
		} else {
			// Successfully processed, delete the message
			_, delErr := c.client.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(c.queueURL),
				ReceiptHandle: msg.ReceiptHandle,
			})
			if delErr != nil {
				log.Printf("[SQS] Failed to delete message %s: %v", *msg.MessageId, delErr)
			}
		}
	}
}

// snsMessage is the wrapper structure if the queue is subscribed to an SNS topic.
type snsMessage struct {
	Message string `json:"Message"`
}

func (c *SQSConsumer) processMessage(msg types.Message) error {
	if msg.Body == nil {
		return nil
	}

	bodyStr := *msg.Body

	if unwrapped, ok := unwrapSNSMessage(bodyStr); ok {
		bodyStr = unwrapped
	}

	var baseEvt struct {
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal([]byte(bodyStr), &baseEvt); err != nil {
		log.Printf("[SQS] Ignoring invalid JSON message %s: %v", *msg.MessageId, err)
		return nil
	}

	eventType := strings.ToUpper(strings.TrimSpace(baseEvt.EventType))
	if eventType == "RIDER_ASSIGNED_TO_ORDER" {
		var evt models.RiderAssignedToOrderEvent
		if err := json.Unmarshal([]byte(bodyStr), &evt); err != nil {
			log.Printf("[SQS] Ignoring invalid JSON message %s: %v", *msg.MessageId, err)
			return nil
		}
		if evt.OrderID == 0 || evt.RestaurantID == 0 || evt.RiderUserID == "" {
			log.Printf("[SQS] Ignoring assignment missing order_id/restaurant_id/rider_user_id")
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		log.Printf("[SQS] Consumed RIDER_ASSIGNED_TO_ORDER order_id=%d restaurant_id=%d rider_id=%s",
			evt.OrderID, evt.RestaurantID, evt.RiderUserID)
		return c.deliverySvc.ProcessRiderAssignedEvent(ctx, &evt)
	}

	var evt models.OrderPlacedEvent
	if err := json.Unmarshal([]byte(bodyStr), &evt); err != nil {
		log.Printf("[SQS] Ignoring invalid JSON message %s: %v", *msg.MessageId, err)
		// Return nil so we delete malformed messages
		return nil
	}

	if eventType == "ORDER_PLACED" {
		if !isDeliveryOrderType(evt.OrderType) {
			log.Printf("[SQS] Ignoring non-delivery order: %d (%s)", evt.OrderID, evt.OrderType)
			return nil
		}

		if evt.OrderID == 0 || evt.RestaurantID == 0 {
			log.Printf("[SQS] Ignoring event missing order_id/restaurant_id")
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		evt.EventType = eventType
		evt.OrderType = "delivery"
		log.Printf("[SQS] Consumed ORDER_PLACED order_id=%d restaurant_id=%d order_type=%s delivery_mode=%s",
			evt.OrderID, evt.RestaurantID, evt.OrderType, evt.DeliveryMode)
		return c.deliverySvc.ProcessOrderPlacedEvent(ctx, &evt)
	}

	return nil
}

func isDeliveryOrderType(orderType string) bool {
	return strings.EqualFold(strings.TrimSpace(orderType), "delivery")
}

func unwrapSNSMessage(body string) (string, bool) {
	var envelope struct {
		Type    string `json:"Type"`
		Message string `json:"Message"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return body, false
	}
	if strings.TrimSpace(envelope.Message) == "" {
		return body, false
	}
	return envelope.Message, true
}

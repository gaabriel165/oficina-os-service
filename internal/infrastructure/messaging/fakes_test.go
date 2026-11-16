package messaging_test

import (
	"context"
	"errors"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

var errQueueUnavailable = errors.New("queue unavailable")

type fakeQueueClient struct {
	mutex          sync.Mutex
	resolveCalls   int
	resolveErr     error
	sendErr        error
	receiveErr     error
	deleteErr      error
	sent           []*sqs.SendMessageInput
	pending        []types.Message
	deletedHandles []string
}

func (f *fakeQueueClient) GetQueueUrl(_ context.Context, input *sqs.GetQueueUrlInput, _ ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.resolveCalls++
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	return &sqs.GetQueueUrlOutput{QueueUrl: aws.String("http://queues/" + aws.ToString(input.QueueName))}, nil
}

func (f *fakeQueueClient) SendMessage(_ context.Context, input *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.sendErr != nil {
		return nil, f.sendErr
	}
	f.sent = append(f.sent, input)
	return &sqs.SendMessageOutput{}, nil
}

func (f *fakeQueueClient) ReceiveMessage(_ context.Context, _ *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.receiveErr != nil {
		return nil, f.receiveErr
	}
	messages := f.pending
	f.pending = nil
	return &sqs.ReceiveMessageOutput{Messages: messages}, nil
}

func (f *fakeQueueClient) DeleteMessage(_ context.Context, input *sqs.DeleteMessageInput, _ ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	f.deletedHandles = append(f.deletedHandles, aws.ToString(input.ReceiptHandle))
	return &sqs.DeleteMessageOutput{}, nil
}

type fakeProcessedMessages struct {
	saved     map[string]bool
	existsErr error
	saveErr   error
}

func newFakeProcessedMessages() *fakeProcessedMessages {
	return &fakeProcessedMessages{saved: map[string]bool{}}
}

func (f *fakeProcessedMessages) Exists(_ context.Context, messageID string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.saved[messageID], nil
}

func (f *fakeProcessedMessages) Save(_ context.Context, messageID string) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved[messageID] = true
	return nil
}

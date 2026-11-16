package messaging

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type queueResolver struct {
	client QueueClient
	mutex  sync.Mutex
	urls   map[string]string
}

func newQueueResolver(client QueueClient) *queueResolver {
	return &queueResolver{client: client, urls: map[string]string{}}
}

func (r *queueResolver) resolve(ctx context.Context, queueName string) (string, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if url, found := r.urls[queueName]; found {
		return url, nil
	}

	output, err := r.client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{QueueName: aws.String(queueName)})
	if err != nil {
		return "", err
	}

	url := aws.ToString(output.QueueUrl)
	r.urls[queueName] = url
	return url, nil
}

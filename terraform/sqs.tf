resource "aws_sqs_queue" "saga_replies_dlq" {
  name                      = "${var.saga_replies_queue_name}-dlq"
  message_retention_seconds = 1209600
  sqs_managed_sse_enabled   = true
}

resource "aws_sqs_queue" "saga_replies" {
  name                       = var.saga_replies_queue_name
  visibility_timeout_seconds = 30
  receive_wait_time_seconds  = 20
  sqs_managed_sse_enabled    = true

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.saga_replies_dlq.arn
    maxReceiveCount     = var.max_receive_count
  })
}

resource "aws_sqs_queue_redrive_allow_policy" "saga_replies_dlq" {
  queue_url = aws_sqs_queue.saga_replies_dlq.id

  redrive_allow_policy = jsonencode({
    redrivePermission = "byQueue"
    sourceQueueArns   = [aws_sqs_queue.saga_replies.arn]
  })
}

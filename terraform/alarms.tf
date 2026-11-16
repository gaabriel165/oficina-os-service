resource "aws_cloudwatch_metric_alarm" "dead_letter_queue_not_empty" {
  alarm_name          = "${var.service_name}-dlq-not-empty"
  alarm_description   = "A saga message failed every delivery attempt and was moved to the dead-letter queue"
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateNumberOfMessagesVisible"
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 1
  threshold           = 0
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    QueueName = aws_sqs_queue.saga_replies_dlq.name
  }
}

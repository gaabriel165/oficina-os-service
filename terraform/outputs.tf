output "saga_replies_queue_url" {
  description = "URL of the queue with the saga replies"
  value       = aws_sqs_queue.saga_replies.id
}

output "saga_replies_dlq_name" {
  description = "Dead-letter queue of the saga replies, watched by the alerts"
  value       = aws_sqs_queue.saga_replies_dlq.name
}

output "pod_role_arn" {
  description = "Role annotated on the Kubernetes service account"
  value       = aws_iam_role.pod.arn
}

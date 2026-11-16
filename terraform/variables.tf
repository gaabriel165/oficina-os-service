variable "region" {
  description = "AWS region to provision into"
  type        = string
  default     = "us-east-1"
}

variable "project_name" {
  description = "Name of the project shared by every repository"
  type        = string
  default     = "oficina-api"
}

variable "service_name" {
  description = "Name prefix applied to the resources of this microservice"
  type        = string
  default     = "oficina-os-service"
}

variable "state_bucket" {
  description = "S3 bucket holding the Terraform state of the other repositories"
  type        = string
  default     = "oficina-api-terraform-state-728750563430"
}

variable "kubernetes_namespace" {
  description = "Namespace of the workload, trusted by the pod role"
  type        = string
  default     = "oficina-api"
}

variable "kubernetes_service_account" {
  description = "Service account of the workload, trusted by the pod role"
  type        = string
  default     = "oficina-api"
}

variable "saga_replies_queue_name" {
  description = "Queue with the replies of the billing and execution microservices, consumed by the saga orchestrator"
  type        = string
  default     = "oficina-saga-replies"
}

variable "command_queue_names" {
  description = "Queues owned by the other microservices that receive the saga commands"
  type        = list(string)
  default     = ["oficina-billing-commands", "oficina-execution-commands"]
}

variable "max_receive_count" {
  description = "Deliveries of a reply before it is moved to the dead-letter queue"
  type        = number
  default     = 5
}

variable "alert_email" {
  description = "E-mail subscribed to the alerts topic. Leave empty to create the topic without subscription"
  type        = string
  default     = ""
}

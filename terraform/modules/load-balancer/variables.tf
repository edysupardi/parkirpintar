variable "project_name" {
  type = string
}

variable "environment" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "public_subnet_ids" {
  type = list(string)
}

variable "private_subnet_ids" {
  type = list(string)
}

variable "alb_security_group_id" {
  type = string
}

variable "certificate_arn" {
  type = string
}

variable "grpc_services" {
  type        = list(string)
  description = "List of gRPC service names to create NLB target groups for"
  default     = ["reservation", "billing", "payment", "presence", "notification"]
}

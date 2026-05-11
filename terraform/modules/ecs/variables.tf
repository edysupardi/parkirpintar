variable "project_name" {
  type = string
}

variable "environment" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "private_subnet_ids" {
  type = list(string)
}

variable "ecs_security_group_id" {
  type = string
}

variable "alb_listener_arn" {
  type = string
}

variable "nlb_arn" {
  type = string
}

variable "gateway_target_group_arn" {
  type = string
}

variable "grpc_target_group_arns" {
  type        = map(string)
  description = "Map of gRPC service name to NLB target group ARN"
}

variable "services" {
  description = "Map of service name to config"
  type = map(object({
    image         = string
    cpu           = number
    memory        = number
    port          = number
    desired_count = number
    grpc          = bool
    environment   = list(object({ name = string, value = string }))
    secrets       = list(object({ name = string, valueFrom = string }))
  }))
}

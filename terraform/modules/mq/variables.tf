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

variable "mq_security_group_id" {
  type = string
}

variable "mq_username" {
  type    = string
  default = "parkirpintar"
}

variable "mq_password" {
  type      = string
  sensitive = true
}

variable "ecs_cluster_id" {
  type = string
}

variable "ecs_execution_role_arn" {
  type = string
}

variable "project_name" {
  type = string
}

variable "environment" {
  type = string
}

variable "ecs_cluster_name" {
  type = string
}

variable "service_names" {
  type = list(string)
}

variable "alb_arn_suffix" {
  type = string
}

variable "rds_cluster_id" {
  type = string
}

variable "elasticache_cluster_id" {
  type = string
}

variable "alarm_email" {
  type = string
}

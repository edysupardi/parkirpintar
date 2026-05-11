variable "aws_region" {
  type    = string
  default = "ap-southeast-1"
}

variable "project_name" {
  type    = string
  default = "edysup-parkirpintar"
}

variable "environment" {
  type    = string
  default = "prod"
}

variable "certificate_arn" {
  type        = string
  description = "ACM certificate ARN for HTTPS"
}

variable "db_password" {
  type      = string
  sensitive = true
}

variable "mq_password" {
  type      = string
  sensitive = true
}

variable "alarm_email" {
  type    = string
  default = "edy@parkirpintar.id"
}

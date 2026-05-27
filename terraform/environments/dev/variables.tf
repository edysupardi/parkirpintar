variable "aws_region" {
  type    = string
  default = "ap-southeast-3"
}

variable "project_name" {
  type    = string
  default = "edysup-parkirpintar"
}

variable "environment" {
  type    = string
  default = "dev"
}

variable "certificate_arn" {
  type        = string
  description = "ACM certificate ARN for HTTPS (optional, HTTP-only if empty)"
  default     = ""
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

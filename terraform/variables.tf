variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "ap-southeast-1"
}

variable "environment" {
  description = "Environment name (dev, prod)"
  type        = string
}

variable "project_name" {
  description = "Project name"
  type        = string
  default     = "edysup-parkirpintar"
}

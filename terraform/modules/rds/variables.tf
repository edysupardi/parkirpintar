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

variable "db_security_group_id" {
  type = string
}

variable "db_password" {
  type      = string
  sensitive = true
}

variable "min_capacity" {
  type    = number
  default = 0.5
}

variable "max_capacity" {
  type    = number
  default = 4
}

locals {
  name        = "${var.project_name}-${var.environment}"
  common_tags = {
    Project     = var.project_name
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

resource "aws_mq_broker" "main" {
  broker_name         = "${local.name}-rabbitmq"
  engine_type         = "RabbitMQ"
  engine_version      = "3.12.13"
  host_instance_type  = "mq.t3.micro"
  deployment_mode     = "SINGLE_INSTANCE"
  publicly_accessible = false

  subnet_ids      = [var.private_subnet_ids[0]]
  security_groups = [var.mq_security_group_id]

  user {
    username = var.mq_username
    password = var.mq_password
  }

  tags = merge(local.common_tags, { Name = "${local.name}-rabbitmq" })
}

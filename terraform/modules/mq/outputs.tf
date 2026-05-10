output "amqp_endpoint" {
  value = "amqp://${var.mq_username}:${var.mq_password}@rabbitmq.${aws_service_discovery_private_dns_namespace.mq.name}:5672/"
}

output "service_discovery_namespace" {
  value = aws_service_discovery_private_dns_namespace.mq.name
}

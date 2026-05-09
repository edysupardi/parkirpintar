output "broker_id" {
  value = aws_mq_broker.main.id
}

output "amqp_endpoint" {
  value = aws_mq_broker.main.instances[0].endpoints[0]
}

output "console_url" {
  value = aws_mq_broker.main.instances[0].console_url
}

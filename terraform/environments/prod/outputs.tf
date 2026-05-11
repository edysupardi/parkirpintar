output "alb_dns_name" {
  value = module.load_balancer.alb_dns_name
}

output "nlb_dns_name" {
  value = module.load_balancer.nlb_dns_name
}

output "rds_endpoint" {
  value = module.rds.cluster_endpoint
}

output "redis_endpoint" {
  value = module.elasticache.endpoint
}

output "mq_amqp_endpoint" {
  value     = module.mq.amqp_endpoint
  sensitive = true
}

output "endpoint" {
  value = aws_elasticache_serverless_cache.main.endpoint[0].address
}

output "port" {
  value = aws_elasticache_serverless_cache.main.endpoint[0].port
}

output "cache_id" {
  value = aws_elasticache_serverless_cache.main.name
}

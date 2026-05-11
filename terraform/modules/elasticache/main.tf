locals {
  name        = "${var.project_name}-${var.environment}"
  common_tags = {
    Project     = var.project_name
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

resource "aws_elasticache_serverless_cache" "main" {
  engine = "redis"
  name   = "${local.name}-redis"

  cache_usage_limits {
    data_storage {
      maximum = 10
      unit    = "GB"
    }
    ecpu_per_second {
      maximum = 5000
    }
  }

  subnet_ids         = var.private_subnet_ids
  security_group_ids = [var.redis_security_group_id]

  tags = merge(local.common_tags, { Name = "${local.name}-redis" })
}

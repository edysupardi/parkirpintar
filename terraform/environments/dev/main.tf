terraform {
  required_version = ">= 1.7"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  backend "s3" {
    bucket         = "edysup-parkirpintar-terraform-state"
    key            = "dev/terraform.tfstate"
    region         = "ap-southeast-1"
    dynamodb_table = "edysup-parkirpintar-terraform-locks"
    encrypt        = true
  }
}

provider "aws" {
  region = var.aws_region
  default_tags {
    tags = {
      Project     = var.project_name
      Environment = var.environment
      Owner       = "edy-supardi"
      ManagedBy   = "terraform"
    }
  }
}

locals {
  services = ["gateway", "reservation", "billing", "payment", "presence", "notification"]
  ecr_base = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.aws_region}.amazonaws.com"
  mq_endpoint = "amqp://${var.project_name}:${var.mq_password}@rabbitmq.${var.project_name}-${var.environment}-mq.local:5672/"
}

data "aws_caller_identity" "current" {}

data "aws_secretsmanager_secret" "jwt" {
  name = "${var.project_name}/dev/jwt-secret"
}

data "aws_secretsmanager_secret" "db_password" {
  name = "${var.project_name}/dev/db-password"
}

data "aws_secretsmanager_secret" "midtrans" {
  name = "${var.project_name}/dev/midtrans-server-key"
}

module "networking" {
  source             = "../../modules/networking"
  project_name       = var.project_name
  environment        = var.environment
  vpc_cidr           = "10.0.0.0/16"
  single_nat_gateway = true
}

module "load_balancer" {
  source                = "../../modules/load-balancer"
  project_name          = var.project_name
  environment           = var.environment
  vpc_id                = module.networking.vpc_id
  public_subnet_ids     = module.networking.public_subnet_ids
  private_subnet_ids    = module.networking.private_subnet_ids
  alb_security_group_id = module.networking.alb_security_group_id
  certificate_arn       = var.certificate_arn
}

module "rds" {
  source               = "../../modules/rds"
  project_name         = var.project_name
  environment          = var.environment
  vpc_id               = module.networking.vpc_id
  private_subnet_ids   = module.networking.private_subnet_ids
  db_security_group_id = module.networking.rds_security_group_id
  db_password          = var.db_password
  min_capacity         = 0.5
  max_capacity         = 4
}

module "elasticache" {
  source                  = "../../modules/elasticache"
  project_name            = var.project_name
  environment             = var.environment
  vpc_id                  = module.networking.vpc_id
  private_subnet_ids      = module.networking.private_subnet_ids
  redis_security_group_id = module.networking.redis_security_group_id
}

module "mq" {
  source                 = "../../modules/mq"
  project_name           = var.project_name
  environment            = var.environment
  vpc_id                 = module.networking.vpc_id
  private_subnet_ids     = module.networking.private_subnet_ids
  mq_security_group_id   = module.networking.mq_security_group_id
  mq_password            = var.mq_password
  ecs_cluster_id         = module.ecs.cluster_id
  ecs_execution_role_arn = module.ecs.execution_role_arn
}

module "ecs" {
  source                   = "../../modules/ecs"
  project_name             = var.project_name
  environment              = var.environment
  vpc_id                   = module.networking.vpc_id
  private_subnet_ids       = module.networking.private_subnet_ids
  ecs_security_group_id    = module.networking.ecs_security_group_id
  alb_listener_arn         = module.load_balancer.alb_https_listener_arn
  nlb_arn                  = module.load_balancer.nlb_arn
  gateway_target_group_arn = module.load_balancer.gateway_target_group_arn
  grpc_target_group_arns   = module.load_balancer.grpc_target_group_arns

  services = {
    gateway = {
      image         = "${local.ecr_base}/${var.project_name}-gateway:latest"
      cpu           = 256
      memory        = 512
      port          = 8080
      desired_count = 1
      grpc          = false
      environment = [
        { name = "ENV", value = "dev" },
        { name = "DB_HOST", value = module.rds.cluster_endpoint },
        { name = "DB_USER", value = "parkirpintar" },
        { name = "REDIS_ADDR", value = "${module.elasticache.endpoint}:${module.elasticache.port}" },
        { name = "REDIS_TLS", value = "true" },
        { name = "RESERVATION_HOST", value = module.load_balancer.nlb_dns_name },
        { name = "RESERVATION_GRPC_PORT", value = "9091" },
        { name = "BILLING_HOST", value = module.load_balancer.nlb_dns_name },
        { name = "BILLING_GRPC_PORT", value = "9092" },
        { name = "PAYMENT_HOST", value = module.load_balancer.nlb_dns_name },
        { name = "PAYMENT_GRPC_PORT", value = "9093" },
        { name = "PRESENCE_HOST", value = module.load_balancer.nlb_dns_name },
        { name = "PRESENCE_GRPC_PORT", value = "9094" },
      ]
      secrets = [
        { name = "JWT_SECRET", valueFrom = data.aws_secretsmanager_secret.jwt.arn },
        { name = "DB_PASSWORD", valueFrom = data.aws_secretsmanager_secret.db_password.arn }
      ]
    }
    reservation = {
      image         = "${local.ecr_base}/${var.project_name}-reservation:latest"
      cpu           = 256
      memory        = 512
      port          = 9090
      desired_count = 1
      grpc          = true
      environment = [
        { name = "ENV", value = "dev" },
        { name = "DB_HOST", value = module.rds.cluster_endpoint },
        { name = "DB_USER", value = "parkirpintar" },
        { name = "REDIS_ADDR", value = "${module.elasticache.endpoint}:${module.elasticache.port}" },
        { name = "REDIS_TLS", value = "true" },
        { name = "MQ_URL", value = local.mq_endpoint },
        { name = "RESERVATION_GRPC_PORT", value = "9090" },
      ]
      secrets = [
        { name = "DB_PASSWORD", valueFrom = data.aws_secretsmanager_secret.db_password.arn },
        { name = "JWT_SECRET", valueFrom = data.aws_secretsmanager_secret.jwt.arn }
      ]
    }
    billing = {
      image         = "${local.ecr_base}/${var.project_name}-billing:latest"
      cpu           = 256
      memory        = 512
      port          = 9090
      desired_count = 1
      grpc          = true
      environment = [
        { name = "ENV", value = "dev" },
        { name = "DB_HOST", value = module.rds.cluster_endpoint },
        { name = "DB_USER", value = "parkirpintar" },
        { name = "REDIS_ADDR", value = "${module.elasticache.endpoint}:${module.elasticache.port}" },
        { name = "REDIS_TLS", value = "true" },
        { name = "BILLING_GRPC_PORT", value = "9090" },
      ]
      secrets = [
        { name = "DB_PASSWORD", valueFrom = data.aws_secretsmanager_secret.db_password.arn },
        { name = "JWT_SECRET", valueFrom = data.aws_secretsmanager_secret.jwt.arn }
      ]
    }
    payment = {
      image         = "${local.ecr_base}/${var.project_name}-payment:latest"
      cpu           = 256
      memory        = 512
      port          = 9090
      desired_count = 1
      grpc          = true
      environment = [
        { name = "ENV", value = "dev" },
        { name = "DB_HOST", value = module.rds.cluster_endpoint },
        { name = "DB_USER", value = "parkirpintar" },
        { name = "REDIS_ADDR", value = "${module.elasticache.endpoint}:${module.elasticache.port}" },
        { name = "REDIS_TLS", value = "true" },
        { name = "PAYMENT_PROVIDER", value = "midtrans" },
        { name = "MIDTRANS_CLIENT_KEY", value = "SB-Mid-client-CteqXD_Bh8T3RYeg" },
        { name = "PAYMENT_GRPC_PORT", value = "9090" },
      ]
      secrets = [
        { name = "DB_PASSWORD", valueFrom = data.aws_secretsmanager_secret.db_password.arn },
        { name = "MIDTRANS_SERVER_KEY", valueFrom = data.aws_secretsmanager_secret.midtrans.arn },
        { name = "JWT_SECRET", valueFrom = data.aws_secretsmanager_secret.jwt.arn }
      ]
    }
    presence = {
      image         = "${local.ecr_base}/${var.project_name}-presence:latest"
      cpu           = 256
      memory        = 512
      port          = 9090
      desired_count = 1
      grpc          = true
      environment = [
        { name = "ENV", value = "dev" },
        { name = "MQ_URL", value = local.mq_endpoint },
        { name = "PRESENCE_GRPC_PORT", value = "9090" },
      ]
      secrets = [
        { name = "JWT_SECRET", valueFrom = data.aws_secretsmanager_secret.jwt.arn },
        { name = "DB_PASSWORD", valueFrom = data.aws_secretsmanager_secret.db_password.arn }
      ]
    }
    notification = {
      image         = "${local.ecr_base}/${var.project_name}-notification:latest"
      cpu           = 256
      memory        = 512
      port          = 9090
      desired_count = 1
      grpc          = true
      environment = [
        { name = "ENV", value = "dev" },
        { name = "MQ_URL", value = local.mq_endpoint },
        { name = "NOTIFICATION_PROVIDER", value = "stub" },
        { name = "NOTIFICATION_GRPC_PORT", value = "9090" },
      ]
      secrets = [
        { name = "JWT_SECRET", valueFrom = data.aws_secretsmanager_secret.jwt.arn },
        { name = "DB_PASSWORD", valueFrom = data.aws_secretsmanager_secret.db_password.arn }
      ]
    }
  }
}

module "monitoring" {
  source                 = "../../modules/monitoring"
  project_name           = var.project_name
  environment            = var.environment
  ecs_cluster_name       = module.ecs.cluster_name
  service_names          = local.services
  alb_arn_suffix         = module.load_balancer.alb_arn_suffix
  rds_cluster_id         = module.rds.cluster_id
  elasticache_cluster_id = module.elasticache.cache_id
  alarm_email            = var.alarm_email
}

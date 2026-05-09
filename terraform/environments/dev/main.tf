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
}

data "aws_caller_identity" "current" {}

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
  source               = "../../modules/mq"
  project_name         = var.project_name
  environment          = var.environment
  vpc_id               = module.networking.vpc_id
  private_subnet_ids   = module.networking.private_subnet_ids
  mq_security_group_id = module.networking.mq_security_group_id
  mq_password          = var.mq_password
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
        { name = "GRPC_RESERVATION_ADDR", value = "${var.project_name}-dev-reservation:9090" },
        { name = "GRPC_BILLING_ADDR", value = "${var.project_name}-dev-billing:9090" },
        { name = "GRPC_PRESENCE_ADDR", value = "${var.project_name}-dev-presence:9090" },
      ]
      secrets = [
        { name = "JWT_SECRET", valueFrom = "arn:aws:secretsmanager:${var.aws_region}:${data.aws_caller_identity.current.account_id}:secret:${var.project_name}/dev/jwt-secret" }
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
        { name = "REDIS_ADDR", value = "${module.elasticache.endpoint}:${module.elasticache.port}" },
        { name = "MQ_URL", value = module.mq.amqp_endpoint },
      ]
      secrets = [
        { name = "DB_PASSWORD", valueFrom = "arn:aws:secretsmanager:${var.aws_region}:${data.aws_caller_identity.current.account_id}:secret:${var.project_name}/dev/db-password" }
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
        { name = "REDIS_ADDR", value = "${module.elasticache.endpoint}:${module.elasticache.port}" },
      ]
      secrets = [
        { name = "DB_PASSWORD", valueFrom = "arn:aws:secretsmanager:${var.aws_region}:${data.aws_caller_identity.current.account_id}:secret:${var.project_name}/dev/db-password" }
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
        { name = "REDIS_ADDR", value = "${module.elasticache.endpoint}:${module.elasticache.port}" },
        { name = "PAYMENT_PROVIDER", value = "midtrans" },
      ]
      secrets = [
        { name = "DB_PASSWORD", valueFrom = "arn:aws:secretsmanager:${var.aws_region}:${data.aws_caller_identity.current.account_id}:secret:${var.project_name}/dev/db-password" },
        { name = "MIDTRANS_SERVER_KEY", valueFrom = "arn:aws:secretsmanager:${var.aws_region}:${data.aws_caller_identity.current.account_id}:secret:${var.project_name}/dev/midtrans-server-key" }
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
        { name = "MQ_URL", value = module.mq.amqp_endpoint },
      ]
      secrets = []
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
        { name = "MQ_URL", value = module.mq.amqp_endpoint },
        { name = "NOTIFICATION_PROVIDER", value = "stub" },
      ]
      secrets = []
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

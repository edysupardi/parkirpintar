locals {
  name        = "${var.project_name}-${var.environment}"
  common_tags = {
    Project     = var.project_name
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

resource "aws_service_discovery_private_dns_namespace" "mq" {
  name = "${local.name}-mq.local"
  vpc  = var.vpc_id
  tags = merge(local.common_tags, { Name = "${local.name}-mq-namespace" })
}

resource "aws_service_discovery_service" "rabbitmq" {
  name = "rabbitmq"

  dns_config {
    namespace_id = aws_service_discovery_private_dns_namespace.mq.id
    dns_records {
      ttl  = 10
      type = "A"
    }
    routing_policy = "MULTIVALUE"
  }

  health_check_custom_config {
    failure_threshold = 1
  }
}

resource "aws_cloudwatch_log_group" "rabbitmq" {
  name              = "/ecs/${local.name}-rabbitmq"
  retention_in_days = 7
  tags              = local.common_tags
}

resource "aws_ecs_task_definition" "rabbitmq" {
  family                   = "${local.name}-rabbitmq"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = var.ecs_execution_role_arn

  container_definitions = jsonencode([
    {
      name  = "rabbitmq"
      image = "rabbitmq:3.13-management-alpine"
      portMappings = [
        { containerPort = 5672, protocol = "tcp" },
        { containerPort = 15672, protocol = "tcp" }
      ]
      environment = [
        { name = "RABBITMQ_DEFAULT_USER", value = var.mq_username },
        { name = "RABBITMQ_DEFAULT_PASS", value = var.mq_password }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.rabbitmq.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "rabbitmq"
        }
      }
    }
  ])

  tags = merge(local.common_tags, { Name = "${local.name}-rabbitmq" })
}

resource "aws_ecs_service" "rabbitmq" {
  name            = "${local.name}-rabbitmq"
  cluster         = var.ecs_cluster_id
  task_definition = aws_ecs_task_definition.rabbitmq.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets         = var.private_subnet_ids
    security_groups = [var.mq_security_group_id]
  }

  service_registries {
    registry_arn = aws_service_discovery_service.rabbitmq.arn
  }

  tags = merge(local.common_tags, { Name = "${local.name}-rabbitmq" })
}

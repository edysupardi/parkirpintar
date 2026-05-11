locals {
  name        = "${var.project_name}-${var.environment}"
  common_tags = {
    Project     = var.project_name
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

resource "aws_ecs_cluster" "main" {
  name = "${local.name}-cluster"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = merge(local.common_tags, { Name = "${local.name}-cluster" })
}

resource "aws_ecs_cluster_capacity_providers" "main" {
  cluster_name       = aws_ecs_cluster.main.name
  capacity_providers = ["FARGATE", "FARGATE_SPOT"]

  default_capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 1
  }
}

# ── IAM roles ────────────────────────────────────────────────────────────────

resource "aws_iam_role" "execution" {
  name = "${local.name}-ecs-execution-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "execution_secrets" {
  name = "${local.name}-execution-secrets"
  role = aws_iam_role.execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue", "ssm:GetParameters"]
      Resource = "*"
    }]
  })
}

resource "aws_iam_role" "task" {
  name = "${local.name}-ecs-task-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy" "task" {
  name = "${local.name}-task-policy"
  role = aws_iam_role.task.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "*"
      },
      {
        Effect   = "Allow"
        Action   = ["xray:PutTraceSegments", "xray:PutTelemetryRecords"]
        Resource = "*"
      },
      {
        Effect   = "Allow"
        Action   = ["ses:SendEmail", "ses:SendRawEmail"]
        Resource = "*"
      }
    ]
  })
}

# ── CloudWatch log groups ─────────────────────────────────────────────────────

resource "aws_cloudwatch_log_group" "services" {
  for_each          = var.services
  name              = "/ecs/${local.name}/${each.key}"
  retention_in_days = 14
  tags              = merge(local.common_tags, { Service = each.key })
}

data "aws_region" "current" {}

# ── Task definitions ──────────────────────────────────────────────────────────

resource "aws_ecs_task_definition" "services" {
  for_each = var.services

  family                   = "${local.name}-${each.key}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = each.value.cpu
  memory                   = each.value.memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  container_definitions = jsonencode([{
    name      = each.key
    image     = each.value.image
    essential = true

    portMappings = [{
      containerPort = each.value.port
      protocol      = "tcp"
      appProtocol   = each.value.grpc ? "grpc" : "http"
    }]

    environment = each.value.environment
    secrets     = each.value.secrets

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = "/ecs/${local.name}/${each.key}"
        "awslogs-region"        = data.aws_region.current.name
        "awslogs-stream-prefix" = "ecs"
      }
    }

    # HTTP health endpoint for gateway; TCP-based health check handled at NLB level for gRPC
    healthCheck = each.value.grpc ? null : {
      command     = ["CMD-SHELL", "wget -qO- http://localhost:${each.value.port}/healthz || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 60
    }
  }])

  tags = merge(local.common_tags, { Service = each.key })
}

# ── ECS Services ──────────────────────────────────────────────────────────────

resource "aws_ecs_service" "gateway" {
  name            = "${local.name}-gateway"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.services["gateway"].arn
  desired_count   = var.services["gateway"].desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets         = var.private_subnet_ids
    security_groups = [var.ecs_security_group_id]
  }

  load_balancer {
    target_group_arn = var.gateway_target_group_arn
    container_name   = "gateway"
    container_port   = var.services["gateway"].port
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }

  tags = merge(local.common_tags, { Service = "gateway" })
}

resource "aws_ecs_service" "grpc" {
  for_each = { for k, v in var.services : k => v if v.grpc }

  name            = "${local.name}-${each.key}"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.services[each.key].arn
  desired_count   = each.value.desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets         = var.private_subnet_ids
    security_groups = [var.ecs_security_group_id]
  }

  load_balancer {
    target_group_arn = var.grpc_target_group_arns[each.key]
    container_name   = each.key
    container_port   = each.value.port
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }

  tags = merge(local.common_tags, { Service = each.key })
}

locals {
  name        = "${var.project_name}-${var.environment}"
  common_tags = {
    Project     = var.project_name
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

# ── ALB (external, HTTPS) ────────────────────────────────────────────────────

resource "aws_lb" "alb" {
  name               = "${local.name}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [var.alb_security_group_id]
  subnets            = var.public_subnet_ids
  tags               = merge(local.common_tags, { Name = "${local.name}-alb" })
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.alb.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.alb.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.certificate_arn

  default_action {
    type = "fixed-response"
    fixed_response {
      content_type = "text/plain"
      message_body = "Not Found"
      status_code  = "404"
    }
  }
}

# ALB target group for gateway service (REST/HTTP)
resource "aws_lb_target_group" "gateway" {
  name        = "${local.name}-gateway-tg"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    path                = "/healthz"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
    matcher             = "200"
  }

  tags = merge(local.common_tags, { Name = "${local.name}-gateway-tg" })
}

# Route all traffic to gateway
resource "aws_lb_listener_rule" "gateway" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 100

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.gateway.arn
  }

  condition {
    path_pattern {
      values = ["/*"]
    }
  }
}

# ── NLB (internal, gRPC) ─────────────────────────────────────────────────────

resource "aws_lb" "nlb" {
  name               = "${local.name}-nlb"
  internal           = true
  load_balancer_type = "network"
  subnets            = var.private_subnet_ids
  tags               = merge(local.common_tags, { Name = "${local.name}-nlb" })
}

# NLB target groups per gRPC service
resource "aws_lb_target_group" "grpc" {
  for_each = toset(var.grpc_services)

  name        = "${local.name}-${each.key}-tg"
  port        = 9090
  protocol    = "TCP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    protocol            = "TCP"
    interval            = 30
    healthy_threshold   = 2
    unhealthy_threshold = 2
  }

  tags = merge(local.common_tags, { Name = "${local.name}-${each.key}-tg" })
}

# NLB listeners per gRPC service (each on a different port)
locals {
  grpc_port_map = {
    reservation  = 9091
    billing      = 9092
    payment      = 9093
    presence     = 9094
    notification = 9095
  }
}

resource "aws_lb_listener" "grpc" {
  for_each = toset(var.grpc_services)

  load_balancer_arn = aws_lb.nlb.arn
  port              = local.grpc_port_map[each.key]
  protocol          = "TCP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.grpc[each.key].arn
  }
}

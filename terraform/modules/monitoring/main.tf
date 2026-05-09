locals {
  name = "${var.project_name}-${var.environment}"
}

# SNS topic for alarms
resource "aws_sns_topic" "alarms" {
  name = "${local.name}-alarms"
}

resource "aws_sns_topic_subscription" "email" {
  topic_arn = aws_sns_topic.alarms.arn
  protocol  = "email"
  endpoint  = var.alarm_email
}

# ECS CPU alarms per service
resource "aws_cloudwatch_metric_alarm" "ecs_cpu" {
  for_each = toset(var.service_names)

  alarm_name          = "${local.name}-${each.key}-cpu-high"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "CPUUtilization"
  namespace           = "AWS/ECS"
  period              = 60
  statistic           = "Average"
  threshold           = 80
  alarm_description   = "ECS CPU > 80% for ${each.key}"
  alarm_actions       = [aws_sns_topic.alarms.arn]

  dimensions = {
    ClusterName = var.ecs_cluster_name
    ServiceName = "${local.name}-${each.key}"
  }
}

# ALB 5xx error rate alarm
resource "aws_cloudwatch_metric_alarm" "alb_5xx" {
  alarm_name          = "${local.name}-alb-5xx-high"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "HTTPCode_ELB_5XX_Count"
  namespace           = "AWS/ApplicationELB"
  period              = 60
  statistic           = "Sum"
  threshold           = 10
  alarm_description   = "ALB 5xx errors > 10 in 1 minute"
  alarm_actions       = [aws_sns_topic.alarms.arn]
  treat_missing_data  = "notBreaching"

  dimensions = {
    LoadBalancer = var.alb_arn_suffix
  }
}

# RDS CPU alarm
resource "aws_cloudwatch_metric_alarm" "rds_cpu" {
  alarm_name          = "${local.name}-rds-cpu-high"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "CPUUtilization"
  namespace           = "AWS/RDS"
  period              = 60
  statistic           = "Average"
  threshold           = 80
  alarm_description   = "RDS CPU > 80%"
  alarm_actions       = [aws_sns_topic.alarms.arn]

  dimensions = {
    DBClusterIdentifier = var.rds_cluster_id
  }
}

# CloudWatch dashboard
resource "aws_cloudwatch_dashboard" "main" {
  dashboard_name = "${local.name}-dashboard"

  dashboard_body = jsonencode({
    widgets = [
      {
        type = "metric"
        properties = {
          title  = "ECS CPU Utilization"
          period = 60
          metrics = [for svc in var.service_names : [
            "AWS/ECS", "CPUUtilization",
            "ClusterName", var.ecs_cluster_name,
            "ServiceName", "${local.name}-${svc}"
          ]]
        }
      },
      {
        type = "metric"
        properties = {
          title  = "ALB Request Count & Errors"
          period = 60
          metrics = [
            ["AWS/ApplicationELB", "RequestCount", "LoadBalancer", var.alb_arn_suffix],
            ["AWS/ApplicationELB", "HTTPCode_ELB_5XX_Count", "LoadBalancer", var.alb_arn_suffix]
          ]
        }
      },
      {
        type = "metric"
        properties = {
          title  = "RDS CPU"
          period = 60
          metrics = [
            ["AWS/RDS", "CPUUtilization", "DBClusterIdentifier", var.rds_cluster_id]
          ]
        }
      }
    ]
  })
}

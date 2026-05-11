output "sns_topic_arn" {
  value = aws_sns_topic.alarms.arn
}

output "dashboard_url" {
  value = "https://console.aws.amazon.com/cloudwatch/home#dashboards:name=${aws_cloudwatch_dashboard.main.dashboard_name}"
}

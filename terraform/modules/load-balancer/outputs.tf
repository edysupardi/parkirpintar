output "alb_arn" {
  value = aws_lb.alb.arn
}

output "alb_dns_name" {
  value = aws_lb.alb.dns_name
}

output "alb_arn_suffix" {
  value = aws_lb.alb.arn_suffix
}

output "alb_https_listener_arn" {
  value = var.certificate_arn != "" ? aws_lb_listener.https[0].arn : aws_lb_listener.http.arn
}

output "alb_http_listener_arn" {
  value = aws_lb_listener.http.arn
}

output "nlb_arn" {
  value = aws_lb.nlb.arn
}

output "nlb_dns_name" {
  value = aws_lb.nlb.dns_name
}

output "gateway_target_group_arn" {
  value = aws_lb_target_group.gateway.arn
}

output "grpc_target_group_arns" {
  value = { for k, v in aws_lb_target_group.grpc : k => v.arn }
}

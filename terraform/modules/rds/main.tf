locals {
  name        = "${var.project_name}-${var.environment}"
  common_tags = {
    Project     = var.project_name
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

resource "aws_db_subnet_group" "main" {
  name       = "${local.name}-db-subnet-group"
  subnet_ids = var.private_subnet_ids
  tags       = merge(local.common_tags, { Name = "${local.name}-db-subnet-group" })
}

resource "aws_rds_cluster" "main" {
  cluster_identifier      = "${local.name}-aurora"
  engine                  = "aurora-postgresql"
  engine_mode             = "provisioned"
  engine_version          = "15.4"
  database_name           = "parkirpintar"
  master_username         = "parkirpintar"
  master_password         = var.db_password
  db_subnet_group_name    = aws_db_subnet_group.main.name
  vpc_security_group_ids  = [var.db_security_group_id]
  skip_final_snapshot     = true
  deletion_protection     = false
  backup_retention_period = 7
  preferred_backup_window = "02:00-03:00"

  serverlessv2_scaling_configuration {
    min_capacity = var.min_capacity
    max_capacity = var.max_capacity
  }

  tags = merge(local.common_tags, { Name = "${local.name}-aurora" })
}

resource "aws_rds_cluster_instance" "main" {
  cluster_identifier = aws_rds_cluster.main.id
  instance_class     = "db.serverless"
  engine             = aws_rds_cluster.main.engine
  engine_version     = aws_rds_cluster.main.engine_version
  tags               = merge(local.common_tags, { Name = "${local.name}-aurora-instance" })
}

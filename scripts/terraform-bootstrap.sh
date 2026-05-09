#!/usr/bin/env bash
# Bootstrap S3 bucket + DynamoDB table for Terraform remote state.
# Run once before `terraform init` in any environment.
# Usage: ./scripts/terraform-bootstrap.sh

set -euo pipefail

BUCKET="edysup-parkirpintar-terraform-state"
TABLE="edysup-parkirpintar-terraform-locks"
REGION="ap-southeast-1"

echo "Creating S3 bucket: $BUCKET"
aws s3api create-bucket \
  --bucket "$BUCKET" \
  --region "$REGION" \
  --create-bucket-configuration LocationConstraint="$REGION"

aws s3api put-bucket-versioning \
  --bucket "$BUCKET" \
  --versioning-configuration Status=Enabled

aws s3api put-bucket-encryption \
  --bucket "$BUCKET" \
  --server-side-encryption-configuration '{
    "Rules": [{
      "ApplyServerSideEncryptionByDefault": {"SSEAlgorithm": "AES256"}
    }]
  }'

aws s3api put-public-access-block \
  --bucket "$BUCKET" \
  --public-access-block-configuration \
    "BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true"

echo "Creating DynamoDB table: $TABLE"
aws dynamodb create-table \
  --table-name "$TABLE" \
  --attribute-definitions AttributeName=LockID,AttributeType=S \
  --key-schema AttributeName=LockID,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --region "$REGION"

echo "Bootstrap complete."
echo "Now run: cd terraform/environments/dev && terraform init"

#!/usr/bin/env bash
# Deploy all ParkirPintar infrastructure (idempotent).
# Usage: ./deploy-infra.sh
#
# Prerequisites:
#   - AWS CLI configured (aws sts get-caller-identity works)
#   - Terraform >= 1.7 installed
#   - Docker running (for image builds)

set -euo pipefail

REGION="ap-southeast-1"
PROJECT="edysup-parkirpintar"
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
ECR="${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com"
CLUSTER="${PROJECT}-dev-cluster"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== ParkirPintar Infrastructure Deploy ==="
echo "Account: $ACCOUNT_ID"
echo "Region:  $REGION"
echo ""

# Step 1: Bootstrap S3 + DynamoDB (skip if exists)
echo "[1/7] Bootstrapping Terraform state..."
if aws s3 ls "s3://${PROJECT}-terraform-state" --region "$REGION" > /dev/null 2>&1; then
  echo "  ✓ S3 bucket exists"
else
  "$ROOT_DIR/scripts/terraform-bootstrap.sh"
fi

# Step 2: OIDC (skip if role exists)
echo "[2/7] Setting up OIDC..."
if aws iam get-role --role-name "${PROJECT}-deploy" > /dev/null 2>&1; then
  echo "  ✓ IAM role exists"
else
  "$ROOT_DIR/scripts/setup-oidc.sh" "$ACCOUNT_ID"
fi

# Step 3: ECR repos (skip if exists)
echo "[3/7] Creating ECR repositories..."
for svc in gateway reservation billing payment presence notification; do
  if aws ecr describe-repositories --repository-names "${PROJECT}-$svc" --region "$REGION" > /dev/null 2>&1; then
    echo "  ✓ ${PROJECT}-$svc exists"
  else
    aws ecr create-repository --repository-name "${PROJECT}-$svc" --region "$REGION" --tags Key=Project,Value="$PROJECT" > /dev/null
    echo "  + ${PROJECT}-$svc created"
  fi
done

# Step 4: Secrets Manager (skip if exists)
echo "[4/7] Creating secrets..."
create_secret() {
  local name=$1 value=$2
  if aws secretsmanager describe-secret --secret-id "$name" --region "$REGION" > /dev/null 2>&1; then
    echo "  ✓ $name exists"
  else
    aws secretsmanager create-secret --name "$name" --secret-string "$value" --region "$REGION" > /dev/null
    echo "  + $name created"
  fi
}

DB_PASS=$(aws secretsmanager get-secret-value --secret-id "${PROJECT}/dev/db-password" --region "$REGION" --query SecretString --output text 2>/dev/null || openssl rand -base64 24 | tr -d '/+=' | head -c 20)
MQ_PASS=$(openssl rand -base64 24 | tr -d '/+=' | head -c 20)
JWT_SECRET=$(openssl rand -base64 32)

create_secret "${PROJECT}/dev/db-password" "$DB_PASS"
create_secret "${PROJECT}/dev/jwt-secret" "$JWT_SECRET"
create_secret "${PROJECT}/dev/midtrans-server-key" "SB-Mid-server-U6zJTc84IGZAHay1Ov22B6kr"

export DB_PASS MQ_PASS

# Step 5: Terraform apply
echo "[5/7] Running Terraform..."
cd "$ROOT_DIR/terraform/environments/dev"
terraform init -input=false
terraform apply -var="db_password=$DB_PASS" -var="mq_password=$MQ_PASS" -auto-approve

ALB_DNS=$(terraform output -raw alb_dns_name)
echo "  ✓ ALB: http://$ALB_DNS"

# Step 6: Build & push Docker images
echo "[6/7] Building and pushing Docker images (linux/amd64)..."
cd "$ROOT_DIR"
aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$ECR"

for svc in gateway reservation billing payment presence notification; do
  echo "  Building $svc..."
  docker build --platform linux/amd64 -f "services/${svc}/Dockerfile" -t "${ECR}/${PROJECT}-${svc}:latest" . > /dev/null 2>&1
  docker push "${ECR}/${PROJECT}-${svc}:latest" > /dev/null 2>&1
  echo "  ✓ $svc pushed"
done

# Step 7: Force deploy all services to latest task definition
echo "[7/7] Deploying services..."
for svc in gateway reservation billing payment presence notification; do
  REV=$(aws ecs describe-task-definition --task-definition "${PROJECT}-dev-$svc" --region "$REGION" --query 'taskDefinition.revision' --output text)
  aws ecs update-service \
    --cluster "$CLUSTER" \
    --service "${PROJECT}-dev-$svc" \
    --task-definition "${PROJECT}-dev-$svc:$REV" \
    --force-new-deployment \
    --desired-count 1 \
    --region "$REGION" \
    --no-cli-pager --output json > /dev/null
  echo "  ✓ $svc → rev $REV"
done

echo ""
echo "=== Deploy complete ==="
echo "ALB endpoint: http://$ALB_DNS"
echo "Health check: curl -s http://$ALB_DNS/healthz"
echo ""
echo "Wait ~2 minutes for services to stabilize, then:"
echo "  aws ecs describe-services --cluster $CLUSTER --services ${PROJECT}-dev-gateway ${PROJECT}-dev-reservation ${PROJECT}-dev-billing ${PROJECT}-dev-payment ${PROJECT}-dev-presence ${PROJECT}-dev-notification --region $REGION --query 'services[].{Name:serviceName,Running:runningCount}' --output table"

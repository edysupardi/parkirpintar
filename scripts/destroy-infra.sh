#!/usr/bin/env bash
# Destroy all ParkirPintar infrastructure.
# Usage: ./destroy-infra.sh

set -euo pipefail

REGION="ap-southeast-1"
PROJECT="edysup-parkirpintar"
CLUSTER="${PROJECT}-dev-cluster"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== ParkirPintar Infrastructure Destroy ==="
echo ""

# Step 1: Scale down all ECS services to 0
echo "[1/3] Scaling down ECS services..."
for svc in gateway reservation billing payment presence notification; do
  aws ecs update-service \
    --cluster "$CLUSTER" \
    --service "${PROJECT}-dev-$svc" \
    --desired-count 0 \
    --region "$REGION" \
    --no-cli-pager --output text > /dev/null 2>&1 && echo "  ✓ $svc scaled to 0" || echo "  - $svc (skip)"
done

echo ""
echo "[2/3] Waiting 30s for tasks to drain..."
sleep 30

# Step 2: Terraform destroy
echo "[3/3] Running terraform destroy..."
cd "$ROOT_DIR/terraform/environments/dev"

if [ -z "${DB_PASS:-}" ] || [ -z "${MQ_PASS:-}" ]; then
  echo "ERROR: DB_PASS and MQ_PASS env vars required."
  echo "Run: export DB_PASS=\$(aws secretsmanager get-secret-value --secret-id $PROJECT/dev/db-password --region $REGION --query SecretString --output text)"
  echo "Run: export MQ_PASS=<your-mq-password>"
  exit 1
fi

terraform destroy \
  -var="db_password=$DB_PASS" \
  -var="mq_password=$MQ_PASS" \
  -auto-approve

echo ""
echo "=== Infrastructure destroyed ==="
echo "Remaining (not managed by Terraform):"
echo "  - ECR repositories (images preserved)"
echo "  - S3 state bucket"
echo "  - DynamoDB lock table"
echo "  - Secrets Manager secrets"
echo "  - IAM OIDC role"
echo ""
echo "To redeploy: ./deploy-infra.sh"

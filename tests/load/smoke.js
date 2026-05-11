import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1'],
  },
};

export default function () {
  const checks = {};

  const health = http.get(`${BASE_URL}/healthz`);
  checks['healthz returns 200'] = health.status === 200;

  const swagger = http.get(`${BASE_URL}/swagger.json`);
  checks['swagger.json returns 200'] = swagger.status === 200;

  const regRes = http.post(`${BASE_URL}/v1/auth/register`, JSON.stringify({
    name: 'Smoke Test',
    email: `smoke-${Date.now()}@test.com`,
    phone: '08100000000',
    password: 'password123',
    vehicle_type: 'car',
  }), { headers: { 'Content-Type': 'application/json' } });
  checks['register returns 201'] = regRes.status === 201;

  if (regRes.status === 201) {
    const token = JSON.parse(regRes.body).token;
    const headers = { headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` } };

    const avail = http.get(`${BASE_URL}/v1/parking/availability?vehicle_type=VEHICLE_TYPE_CAR`, headers);
    checks['availability returns 200'] = avail.status === 200;

    const spots = http.get(`${BASE_URL}/v1/parking/spots?vehicle_type=car&floor=1`, headers);
    checks['list spots returns 200'] = spots.status === 200;

    const history = http.get(`${BASE_URL}/v1/reservations/history`, headers);
    checks['history returns 200'] = history.status === 200;
  }

  check(null, checks);
}

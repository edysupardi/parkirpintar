import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

const errorRate = new Rate('errors');
const reservationDuration = new Trend('reservation_duration');

export const options = {
  scenarios: {
    smoke: {
      executor: 'constant-vus',
      vus: 1,
      duration: '10s',
      exec: 'smokeTest',
    },
    load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 10 },
        { duration: '1m', target: 10 },
        { duration: '30s', target: 0 },
      ],
      exec: 'loadTest',
      startTime: '15s',
    },
    stress: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 30 },
        { duration: '1m', target: 30 },
        { duration: '30s', target: 0 },
      ],
      exec: 'loadTest',
      startTime: '2m30s',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
    errors: ['rate<0.05'],
  },
};

function getToken() {
  const res = http.post(`${BASE_URL}/v1/auth/register`, JSON.stringify({
    name: `driver-${Date.now()}-${Math.random().toString(36).slice(2)}`,
    email: `driver-${Date.now()}-${Math.random().toString(36).slice(2)}@test.com`,
    phone: `081${Math.floor(Math.random() * 1000000000)}`,
    password: 'password123',
    vehicle_type: 'car',
  }), { headers: { 'Content-Type': 'application/json' } });

  if (res.status === 201) {
    return JSON.parse(res.body).token;
  }
  return null;
}

function authHeaders(token) {
  return {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
  };
}

export function smokeTest() {
  const healthRes = http.get(`${BASE_URL}/healthz`);
  check(healthRes, { 'healthz 200': (r) => r.status === 200 });

  const token = getToken();
  if (!token) {
    errorRate.add(1);
    return;
  }

  const availRes = http.get(`${BASE_URL}/v1/parking/availability?vehicle_type=VEHICLE_TYPE_CAR`, authHeaders(token));
  check(availRes, { 'availability 200': (r) => r.status === 200 });
  errorRate.add(availRes.status !== 200);
}

export function loadTest() {
  const token = getToken();
  if (!token) {
    errorRate.add(1);
    return;
  }

  const headers = authHeaders(token);

  // check availability
  const availRes = http.get(`${BASE_URL}/v1/parking/availability?vehicle_type=VEHICLE_TYPE_CAR`, headers);
  check(availRes, { 'availability 200': (r) => r.status === 200 });

  // create reservation
  const idempotencyKey = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  const start = Date.now();
  const resRes = http.post(`${BASE_URL}/v1/reservations`, JSON.stringify({
    vehicle_type: 'VEHICLE_TYPE_CAR',
    assignment_mode: 'ASSIGNMENT_MODE_SYSTEM',
    idempotency_key: idempotencyKey,
  }), headers);

  reservationDuration.add(Date.now() - start);

  if (resRes.status === 200) {
    const reservation = JSON.parse(resRes.body);
    const resId = reservation.reservation_id;

    // check-in
    const checkInRes = http.post(`${BASE_URL}/v1/reservations/${resId}/check-in`, JSON.stringify({}), headers);
    check(checkInRes, { 'check-in 200': (r) => r.status === 200 });

    // check-out
    const checkOutRes = http.post(`${BASE_URL}/v1/reservations/${resId}/check-out`, JSON.stringify({
      idempotency_key: `co-${idempotencyKey}`,
    }), headers);
    check(checkOutRes, { 'check-out 200': (r) => r.status === 200 });

    errorRate.add(0);
  } else {
    // spot unavailable is expected under load
    errorRate.add(resRes.status !== 200 && resRes.status !== 429);
  }

  sleep(1);
}

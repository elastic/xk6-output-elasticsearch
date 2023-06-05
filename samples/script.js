import http from "k6/http";
import { check } from "k6";

export const options = {
  vus: 5,
  duration: '1m',
  thresholds: {
    'http_reqs{expected_response:true}': ['rate>5'],
  },
};

export default function () {
  check(http.get("https://httpbin.test.k6.io/status/200"), {
    "status is 200": (r) => r.status == 200,
  });
}

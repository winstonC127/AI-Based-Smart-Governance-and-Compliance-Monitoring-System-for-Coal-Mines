/**
 * api.js — thin fetch wrapper around the Go backend REST API.
 * Every response from the backend follows { success, message, data, error }.
 * This module normalizes that so callers can just `await api.get(...)`.
 */
const API = (() => {
  const BASE_URL = window.APP_CONFIG?.API_BASE_URL || 'http://localhost:8080/api';

  function getToken() {
    return localStorage.getItem('cg_token');
  }

  async function request(method, path, body) {
    const headers = { 'Content-Type': 'application/json' };
    const token = getToken();
    if (token) headers['Authorization'] = `Bearer ${token}`;

    let res;
    try {
      res = await fetch(`${BASE_URL}${path}`, {
        method,
        headers,
        body: body ? JSON.stringify(body) : undefined,
      });
    } catch (networkErr) {
      throw new Error('Cannot reach the server. Check that the backend is running.');
    }

    let json;
    try {
      json = await res.json();
    } catch (parseErr) {
      throw new Error(`Unexpected server response (status ${res.status}).`);
    }

    if (res.status === 401) {
      // Session expired or invalid — force re-login.
      localStorage.removeItem('cg_token');
      localStorage.removeItem('cg_user');
      if (!location.pathname.endsWith('login.html')) {
        location.href = 'login.html?expired=1';
      }
      throw new Error(json.message || 'Session expired');
    }

    if (!json.success) {
      throw new Error(json.message || 'Request failed');
    }

    return json.data;
  }

  return {
    get: (path) => request('GET', path),
    post: (path, body) => request('POST', path, body),
    put: (path, body) => request('PUT', path, body),
    delete: (path, body) => request('DELETE', path, body),
    del: (path, body) => request('DELETE', path, body),
  };
})();


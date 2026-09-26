/**
 * config.js — runtime configuration for CoalGuard AI frontend.
 * Resolves the backend API endpoint dynamically:
 * 1. Query parameter: ?api=... (e.g., for ad-hoc testing/switching)
 * 2. localStorage: 'API_BASE_URL' (persisted override)
 * 3. Localhost: http://localhost:8080/api
 * 4. Production: https://ai-based-smart-governance-and-compliance-fc8y.onrender.com/api
 */
(function() {
  try {
    const urlParams = new URLSearchParams(window.location.search);
    const paramApi = urlParams.get('api');
    if (paramApi) {
      localStorage.setItem('API_BASE_URL', paramApi.replace(/\/+$/, ''));
    }
  } catch (e) {}

  const isLocal = window.location.hostname === 'localhost' || 
                  window.location.hostname === '127.0.0.1' || 
                  window.location.hostname === '';
  
  let savedApi = null;
  try {
    savedApi = localStorage.getItem('API_BASE_URL');
  } catch (e) {}

  const defaultProdApi = 'https://ai-based-smart-governance-and-compliance-fc8y.onrender.com/api';

  window.APP_CONFIG = {
    API_BASE_URL: (savedApi || (isLocal ? 'http://localhost:8080/api' : defaultProdApi)).replace(/\/+$/, ''),
  };
})();

// Global authentication utility for handling 401 responses

let authCallback: (() => void) | null = null;

// Set the callback to be called when authentication fails
export function setAuthFailureCallback(callback: () => void) {
  authCallback = callback;
}

// Handle 401 responses globally - MUST be called whenever a 401 is received
export function handle401() {
  // First clear the auth state through the callback
  if (authCallback) {
    authCallback();
  }
  
  // Force immediate redirect - don't wait
  // Use location.href to ensure full page reload
  window.location.href = '/auth/login';
}

// Wrapper around fetch that handles 401 globally
export async function authFetch(url: string, options?: RequestInit): Promise<Response> {
  const response = await fetch(url, {  ...options, 
    credentials: options?.credentials || 'include' 
  });
  
  if (response.status === 401) {
    handle401();
  }
  
  return response;
}

// Check if a response is 401 and handle it
export function checkAuth(response: Response): Response {
  if (response.status === 401) {
    handle401();
  }
  return response;
}
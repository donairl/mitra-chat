// Shared axios instance for all REST calls. All endpoint wrappers in index.ts
// build on this so auth handling and error handling live in one place.
import axios from 'axios'

// baseURL '/api' is proxied to the Go backend (see vite config / dev proxy).
const client = axios.create({ baseURL: '/api' })

// Request interceptor: attach the stored JWT as a Bearer token on every call.
client.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Response interceptor: on a 401 with a token present, the token is stale/invalid
// — clear it and bounce to /login (unless already there, to avoid a redirect loop).
client.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401 && localStorage.getItem('token')) {
      localStorage.removeItem('token')
      if (location.pathname !== '/login') location.href = '/login'
    }
    return Promise.reject(err)
  },
)

export default client

import { afterEach, describe, expect, it, vi } from 'vitest'
import { AxiosError, type AxiosResponse } from 'axios'
import { apiError, apiStatus, safeRefetch } from '@/api'

// An axios error as the client sees it for an HTTP error response.
const httpError = (status: number, data: unknown) =>
  new AxiosError('failed', 'ERR_BAD_REQUEST', undefined, undefined, {
    status,
    data,
  } as AxiosResponse)

describe('apiError', () => {
  it("returns the server's error message", () => {
    expect(apiError(httpError(403, { error: 'insufficient permissions' }))).toBe(
      'insufficient permissions',
    )
  })

  it('falls back when there is no message, no response or no axios error', () => {
    expect(apiError(httpError(500, {}), 'Failed')).toBe('Failed')
    expect(apiError(httpError(502, '<html>bad gateway</html>'), 'Failed')).toBe('Failed')
    expect(apiError(new AxiosError('Network Error'), 'Failed')).toBe('Failed')
    expect(apiError(new Error('boom'), 'Failed')).toBe('Failed')
    expect(apiError(undefined)).toBe('Something went wrong')
  })
})

describe('apiStatus', () => {
  it('reads the HTTP status and is undefined for anything else', () => {
    expect(apiStatus(httpError(403, {}))).toBe(403)
    expect(apiStatus(new AxiosError('Network Error'))).toBeUndefined()
    expect(apiStatus(new Error('boom'))).toBeUndefined()
  })
})

describe('safeRefetch', () => {
  afterEach(() => vi.restoreAllMocks())

  it('swallows a 403 quietly and logs other failures', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    safeRefetch(Promise.reject(httpError(403, { error: 'not a member' })))
    safeRefetch(Promise.reject(httpError(404, { error: 'channel not found' })))
    await new Promise((r) => setTimeout(r))
    expect(warn).not.toHaveBeenCalled()

    safeRefetch(Promise.reject(new AxiosError('Network Error')))
    await new Promise((r) => setTimeout(r))
    expect(warn).toHaveBeenCalledOnce()
  })
})

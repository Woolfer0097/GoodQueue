export class ApiError extends Error {
  readonly data: unknown;
  readonly status: number;
  readonly statusText: string;

  constructor(response: Response, data: unknown) {
    const statusDescription = `${response.status} ${response.statusText}`.trim();

    super(`HTTP request failed: ${statusDescription}`);
    this.name = 'ApiError';
    this.data = data;
    this.status = response.status;
    this.statusText = response.statusText;
  }
}

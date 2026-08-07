import { ApiError } from './api-error';

type Fetch = typeof fetch;

const buildUrl = (baseUrl: string, path: string) =>
  `${baseUrl.replace(/\/+$/, '')}/${path.replace(/^\/+/, '')}`;

const readResponseData = async (response: Response): Promise<unknown> => {
  if (response.status === 204) {
    return undefined;
  }

  const body = await response.text();

  if (body.length === 0) {
    return undefined;
  }

  if (!response.headers.get('Content-Type')?.includes('application/json')) {
    return body;
  }

  try {
    return JSON.parse(body) as unknown;
  } catch {
    return body;
  }
};

export const createApiClient =
  (baseUrl: string, fetchImplementation: Fetch = fetch) =>
  async <ResponseData = unknown>(path: string, init?: RequestInit): Promise<ResponseData> => {
    const response = await fetchImplementation(buildUrl(baseUrl, path), init);
    const data = await readResponseData(response);

    if (!response.ok) {
      throw new ApiError(response, data);
    }

    return data as ResponseData;
  };

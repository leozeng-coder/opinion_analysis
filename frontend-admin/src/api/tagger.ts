import request from './request'

export const taggerApi = {
  pending: () => request.get<never, { pending: number }>('/tagger/pending'),
  run: () => request.post<never, { message: string }>('/tagger/run'),
}

import { Request, Response, NextFunction } from 'express'
import { obs } from '../observability.js'

export function httpLogger(req: Request, res: Response, next: NextFunction) {
  const start = Date.now()
  res.on('finish', () => {
    obs.request(req.method, req.path, res.statusCode, Date.now() - start)
  })
  next()
}

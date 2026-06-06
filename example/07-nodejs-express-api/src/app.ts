import express from 'express'
import { httpLogger } from './middleware/httpLogger.js'
import tasksRouter from './routes/tasks.js'
import healthRouter from './routes/health.js'

export const app = express()

app.use(express.json())
app.use(httpLogger)
app.use('/health', healthRouter)
app.use('/tasks', tasksRouter)

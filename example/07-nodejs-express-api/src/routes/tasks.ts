import { Router } from 'express'
import { randomUUID } from 'node:crypto'
import { obs } from '../observability.js'
import type { Task } from '../types.js'

const router = Router()
const tasks = new Map<string, Task>()

router.get('/', (_req, res) => {
  res.json({ tasks: [...tasks.values()] })
})

router.post('/', (req, res) => {
  const { title } = req.body as { title?: unknown }
  if (!title || typeof title !== 'string') {
    res.status(400).json({ error: 'title is required' })
    return
  }
  const task: Task = {
    id: randomUUID(),
    title,
    done: false,
    createdAt: new Date().toISOString(),
  }
  tasks.set(task.id, task)
  obs.info(`Task created: "${title}"`, 'tasks')
  res.status(201).json(task)
})

router.patch('/:id/done', (req, res) => {
  const task = tasks.get(req.params.id)
  if (!task) {
    res.status(404).json({ error: 'not found' })
    return
  }
  task.done = true
  obs.info(`Task completed: "${task.title}"`, 'tasks')
  res.json(task)
})

router.delete('/:id', (req, res) => {
  const task = tasks.get(req.params.id)
  if (!task) {
    res.status(404).json({ error: 'not found' })
    return
  }
  tasks.delete(req.params.id)
  obs.warn(`Task deleted: "${task.title}"`, 'tasks')
  res.status(204).send()
})

export default router

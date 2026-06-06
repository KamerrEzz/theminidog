import { Router, Response } from 'express'
import { prisma } from '../lib/prisma.js'
import { obs } from '../observability.js'
import { requireAuth, AuthRequest } from '../middleware/auth.js'

const router = Router()
router.use(requireAuth)

router.get('/', async (req: AuthRequest, res: Response) => {
  const tasks = await prisma.task.findMany({
    where: { userId: req.userId! },
    orderBy: { createdAt: 'desc' },
  })
  res.json({ tasks })
})

router.post('/', async (req: AuthRequest, res: Response) => {
  const { title } = req.body
  if (!title || typeof title !== 'string') {
    return res.status(400).json({ error: 'title is required' })
  }
  const task = await prisma.task.create({
    data: { title, userId: req.userId! },
  })
  obs.info(`Task created: "${title}"`, 'tasks')
  res.status(201).json(task)
})

router.patch('/:id/done', async (req: AuthRequest, res: Response) => {
  const task = await prisma.task.findUnique({ where: { id: req.params.id } })
  if (!task) return res.status(404).json({ error: 'not found' })
  if (task.userId !== req.userId) return res.status(403).json({ error: 'forbidden' })
  const updated = await prisma.task.update({ where: { id: req.params.id }, data: { done: true } })
  obs.info(`Task completed: "${task.title}"`, 'tasks')
  res.json(updated)
})

router.delete('/:id', async (req: AuthRequest, res: Response) => {
  const task = await prisma.task.findUnique({ where: { id: req.params.id } })
  if (!task) return res.status(404).json({ error: 'not found' })
  if (task.userId !== req.userId) return res.status(403).json({ error: 'forbidden' })
  await prisma.task.delete({ where: { id: req.params.id } })
  obs.warn(`Task deleted: "${task.title}"`, 'tasks')
  res.status(204).send()
})

export default router

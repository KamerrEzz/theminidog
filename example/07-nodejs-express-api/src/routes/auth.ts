import { Router } from 'express'
import bcrypt from 'bcryptjs'
import jwt from 'jsonwebtoken'
import { prisma } from '../lib/prisma.js'
import { obs } from '../observability.js'

const router = Router()
const SALT_ROUNDS = 10

router.post('/register', async (req, res) => {
  const { name, email, password } = req.body
  if (!name || !email || !password) {
    return res.status(400).json({ error: 'name, email and password are required' })
  }
  try {
    const hashed = await bcrypt.hash(password, SALT_ROUNDS)
    const user = await prisma.user.create({
      data: { name, email, password: hashed },
      select: { id: true, name: true, email: true, createdAt: true },
    })
    obs.info(`User registered: ${email}`, 'auth')
    res.status(201).json(user)
  } catch (err: any) {
    if (err.code === 'P2002') {
      return res.status(409).json({ error: 'email already in use' })
    }
    obs.error(`Register failed: ${err.message}`, 'auth')
    res.status(500).json({ error: 'internal error' })
  }
})

router.post('/login', async (req, res) => {
  const { email, password } = req.body
  if (!email || !password) {
    return res.status(400).json({ error: 'email and password are required' })
  }
  const user = await prisma.user.findUnique({ where: { email } })
  if (!user || !(await bcrypt.compare(password, user.password))) {
    return res.status(401).json({ error: 'invalid credentials' })
  }
  const token = jwt.sign({ sub: user.id }, process.env.JWT_SECRET ?? 'dev-secret', { expiresIn: '7d' })
  obs.info(`User logged in: ${email}`, 'auth')
  res.json({ token, user: { id: user.id, name: user.name, email: user.email } })
})

export default router

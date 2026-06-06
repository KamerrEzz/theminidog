import { app } from './app.js'
import { obs } from './observability.js'

const PORT = parseInt(process.env.PORT ?? '3000', 10)

app.listen(PORT, () => {
  obs.info(`Tasks API listening on :${PORT}`, 'startup')
  console.log(`→ http://localhost:${PORT}`)
})

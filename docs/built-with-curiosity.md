# Built with Curiosity

This project started with a simple question: *"I've heard of Datadog. How does it actually work?"*

Not "how do I use it" — how does it *work*. What happens when a metric leaves a server and ends up on a dashboard somewhere? How does a system collect CPU usage from thousands of machines and make sense of it in real time? I wanted to understand it, not just use it.

Datadog had a paid plan. I didn't dig further. Instead I thought: *what if I just build my own version?*

That's where this started.

---

## Who built this

My name is KamerrEzz. I'm a full-stack developer from Mexico, mostly working in the SaaS space — web platforms, subscriptions, dashboards, that kind of thing. JavaScript and TypeScript are my home. I also write Lua when it comes up.

Go was new to me when this project started. I was considering .NET for a while, but something about Go clicked — it felt like a language that could give me what JavaScript couldn't without the overhead of learning an entirely different ecosystem. I knew JavaScript, I knew Lua. Go felt like the right next step for the kind of systems work I wanted to explore.

Spoiler: I liked Go a lot.

---

## What AI actually did here

I want to be honest about this because I think there's a version of this story that makes it sound like I just told an AI to build a project and it appeared. That's not what happened.

This was a 50/50 collaboration.

AI helped me understand things I didn't know. When I needed to collect CPU metrics from a Linux system, I didn't know where that data even lived. AI explained `/proc/stat`, showed me the delta calculation, helped me understand why network metrics need to be diffed between samples. When I was learning Go concurrency patterns — channels, goroutines, context cancellation — AI helped me understand the *why* behind the patterns, not just the syntax.

I was the one making decisions. I chose the architecture. I decided what to build each week. I reviewed the code. I caught bugs. I pushed back when something felt wrong. When the alerting implementation had a quirk — firing "resolved" on the very first evaluation of a new rule — I noticed it and we fixed it.

What would have taken me four to six months to build alone — learning Go, understanding observability systems, writing 213 tests — happened in five weeks. Not because AI wrote it for me. Because AI compressed the learning curve dramatically. I spent my time on decisions and understanding, not on figuring out how to parse a duration string.

There's a difference between using AI as a shortcut and using AI as a teacher. This was the second one.

---

## The moment I decided to take it seriously

Somewhere around Week 2, I realized this wasn't just a throwaway experiment. The code was clean. The tests passed. The architecture actually made sense. I started thinking: *what if someone else could use this?*

That's when the SDK appeared. I mentioned wanting to have a TypeScript client — just for myself. By the end of the conversation, there was a published npm package. I hadn't planned that. The process of building properly led naturally to shipping properly.

The Docker Hub images came from the same place. The documentation site. The bilingual docs. The GitHub Actions workflows. None of these were in the original plan. They happened because once you start building something real, the next right step becomes obvious.

---

## What Zeew Space has to do with this

I run [Zeew Space](https://zeew.space) — a platform where you learn to code by building real things from day one. Not endless theory videos. The model is: understand a concept, apply it immediately in code, get feedback from a real person, and finish with a project you can show in your portfolio.

It works through routes — progressive paths where each course is the foundation for the next. You learn JavaScript, then React, then Next.js, and at the end of each stage you have something functional that's yours. AI is integrated as a tool that accelerates what you already know — not as a crutch.

The difference: other platforms teach you to program. Zeew teaches you to think like a programmer, then teaches you the syntax.

If you want to learn programming — or go deeper into systems, AI-assisted development, or building real products:

**[zeew.space/discord](https://zeew.space/discord)** — join the community, get feedback on what you build, and learn alongside people who are doing the same thing.

This project is one example of what learning by building actually looks like.

---

## What I'd tell someone starting from zero

If you want to build something like this — a real system, not a tutorial project — here's the honest version:

**You don't need to know everything first.** I didn't know Go when I started. I didn't know how CPU metrics were collected. I didn't know how TimescaleDB time-bucket queries worked. I learned all of it during the project, not before it.

**Pick something you actually want to understand.** Not a todo app. Not a clone of something you've already built. Pick a system you use but don't understand. That curiosity is the fuel. Without it, you'll stop when it gets hard.

**Use AI as a teacher, not a ghostwriter.** Ask it to explain things. Ask it why. When it suggests something, understand it before you use it. The moment you stop understanding your own code, you've lost the learning.

**Ship something.** The act of making it public — even one feature, even imperfect — changes how you build. You start caring about README files and error messages and what happens when someone else runs your code for the first time. That's where the real learning is.

---

## Final thought

I don't know what I'll use Go for next. I don't know if I'll need a monitoring system for something real. But I understand how one works now — from the agent collecting metrics every ten seconds to the hypertable storing them to the SVG sparkline rendering in the browser.

That understanding doesn't expire.

---

*KamerrEzz — Zeew Space · [GitHub](https://github.com/KamerrEzz) · [Discord](https://zeew.space/discord)*

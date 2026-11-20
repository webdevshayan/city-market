# City Market

A **100% Halal delivery marketplace**: customers order from certified-Halal shops, riders deliver, and every fee in the system follows the fixed business rules below. Nothing in this platform handles riba (interest): commissions are flat tiered service fees (Ujrah), rider pay is a fixed per-delivery + per-km wage, and the customer fee is a flat checkout charge.

## Layout

```
apps/citizen   Flutter customer app
apps/shop      Flutter shop dashboard
apps/rider     Flutter rider app
apps/admin     Flutter admin console
server         Go HTTP server (API + web, deployed on Vercel)
web            Tailwind build that emits ONE minified stylesheet
docs           Phase notes and decisions
```

The repository is built in phases; each phase lands as a pull request with a description of what changed, how it was tested, and what to verify next.

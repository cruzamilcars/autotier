# Replica abierta de Cursor Router Auto (Cost/Balance/Intelligence).
# Cursor nativo solo en Teams/Enterprise; esto lo lleva a cualquier repo.
# Regla: subagentes con model: inherit siguen al chat; overrea con modelo fijo solo donde ahorra.

# .cursor/agents/plan.md -> model: opus/gpt-5-high (frontera solo plan)
# .cursor/agents/explore.md -> model: inherit (barato, sigue al padre barato)
# .cursor/agents/implement.md -> model: sonnet/gpt-5 (equilibrado)
# Modo diario: Auto/Balance. Escala a Intelligence solo en arquitectura/seguridad.

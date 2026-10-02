export const queryKeys = {
  principal: (principal: string) => ['principal', principal] as const,
  delegations: (principal: string) => ['principal', principal, 'delegations'] as const,
  agent: (principal: string, id: string) => ['principal', principal, 'agent', id] as const,
  grant: (principal: string, id: string) => ['principal', principal, 'grant', id] as const,
  decision: (principal: string, id: string, token: string) =>
    ['principal', principal, 'agent-decision', id, token] as const,
  connections: (principal: string) => ['principal', principal, 'connections'] as const,
  pending: (principal: string) => ['principal', principal, 'pending'] as const,
  standing: (principal: string) => ['principal', principal, 'standing'] as const,
  approval: (principal: string, id: string) => ['principal', principal, 'approval', id] as const,
};

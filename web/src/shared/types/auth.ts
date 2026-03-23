export interface AuthSession {
  userId: string
  sessionId: string
  token: string
}

export interface LoginInput {
  username: string
  password: string
}

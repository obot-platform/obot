export class ConflictError extends Error {}
export class BadRequestError extends Error {}
export class NotFoundError extends Error {}

// Errors that should trigger the error boundary
export class BoundaryError extends Error {}

export class ForbiddenError extends BoundaryError {}
export class UnauthorizedError extends BoundaryError {}

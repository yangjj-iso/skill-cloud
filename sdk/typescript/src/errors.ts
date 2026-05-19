/** Error raised when the Skill Cloud API returns a non-2xx response. */
export class SkillCloudError extends Error {
  public readonly statusCode: number;

  constructor(statusCode: number, message: string) {
    super(`skill-cloud API error ${statusCode}: ${message}`);
    this.name = "SkillCloudError";
    this.statusCode = statusCode;
  }
}

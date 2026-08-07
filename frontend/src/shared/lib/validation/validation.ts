import { z } from 'zod';

export const nonEmptyStringSchema = z.string().trim().min(1);

export const uuidSchema = z.uuid();

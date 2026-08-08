import { AspectRatio, Skeleton, Stack } from '@mantine/core';

export function ProductCardSkeleton() {
  return (
    <Stack gap="xs">
      <AspectRatio ratio={1}>
        <Skeleton radius="md" />
      </AspectRatio>
      <Skeleton height={16} width="80%" />
      <Skeleton height={22} width="45%" />
      <Skeleton height={16} width="55%" />
      <Skeleton height={14} width="70%" />
    </Stack>
  );
}

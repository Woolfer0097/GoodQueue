import { Button, Center, Container, Stack, Text, Title, VisuallyHidden } from '@mantine/core';
import { Link } from 'react-router';

export function NotFoundPage() {
  return (
    <Container size="xl" py={{ base: 'xl', sm: 80 }}>
      <Center mih="60vh">
        <Stack align="center" gap="md" maw={640} ta="center">
          <VisuallyHidden>404</VisuallyHidden>

          <Title c="avitoBlue.7" fz={{ base: 28, sm: 36 }} lh={1.15} order={1}>
            Такой страницы не существует
          </Title>

          <Text c="gray.6" fz={{ base: 'sm', sm: 'md' }}>
            Возможно, страницу удалили или в ссылке есть опечатка.
          </Text>

          <Button
            color="avitoBlue"
            component={Link}
            mt="sm"
            radius="md"
            size="md"
            to="/"
            variant="light"
          >
            Вернуться в каталог
          </Button>
        </Stack>
      </Center>
    </Container>
  );
}

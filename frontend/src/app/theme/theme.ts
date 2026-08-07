import { createTheme } from '@mantine/core';

export const theme = createTheme({
  colors: {
    avitoBlue: [
      '#e6f7ff',
      '#cceeff',
      '#99ddff',
      '#66ccff',
      '#33bbff',
      '#1ab3ff',
      '#00aaff',
      '#0099e6',
      '#0088cc',
      '#0077b3',
    ],
  },
  defaultRadius: 'md',
  fontFamily: 'Arial, sans-serif',
  primaryColor: 'avitoBlue',
  radius: {
    xs: '4px',
    sm: '8px',
    md: '12px',
    lg: '16px',
    xl: '24px',
  },
});

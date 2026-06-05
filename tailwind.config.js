/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./views/**/*.html",
    "./views/partials/**/*.html"
  ],
  theme: {
    extend: {
      colors: {
        green: '#006633',
        green_dark: '#004d26',
        green_light: '#008844',
        green_pale: '#e8f5ee',
        green_mid: '#cceadb',
        red: '#8C1515',
        red_light: '#b01e1e',
        red_pale: '#fdf0f0',
        gold: '#c9a84c',
        gold_light: '#e8c96e',
        ivory: '#faf8f4',
        charcoal: '#1a1a1a',
        slate: '#3d4852',
        muted: '#6b7280',
        border: '#d1d5db',
        white: '#ffffff',
      },
      fontFamily: {
        sans: ['DM Sans'],
        serif: ['Playfair Display'],
        amiri: ['Amiri'],
      },
    },
  },
  plugins: [],
}


/** @type {import('tailwindcss').Config} */
const { colors } = require("tailwindcss/defaultTheme");
module.exports = {
  content: ["./views/*.{html,templ}"],
  theme: {
    extend: {
      colors: {
        background: "#121212", // dark background
        surface: "#1F2937", // surface background
        primary: "#DC2626", // bright red
        secondary: "#374151", // soft gray
        maintext: "#F9FAFB", // main text
      },
    },
  },
  plugins: [],
};

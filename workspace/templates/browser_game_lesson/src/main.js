const app = document.querySelector("#app");

app.innerHTML = `
  <h1>Sort the examples</h1>
  <button data-answer="solid">Solid</button>
  <button data-answer="liquid">Liquid</button>
  <button data-answer="gas">Gas</button>
  <p id="feedback"></p>
`;

app.addEventListener("click", (event) => {
  const answer = event.target.dataset.answer;
  if (!answer) return;
  document.querySelector("#feedback").textContent = `Selected ${answer}`;
});

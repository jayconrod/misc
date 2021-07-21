let matchers = [
  githubMatcher,
  gerritMatcher
];

async function noteLink() {
  let [tab] = await chrome.tabs.query({active: true, currentWindow: true});

  // Decide on link title and description.
  let link = tab.title;
  let description = "";
  for (let matcher of matchers) {
    let match = matcher(tab);
    if (match) {
      link = match.link;
      description = match.description;
      break;
    }
  }

  // Create HTML to copy.
  let div = document.createElement("div");
  let a = document.createElement("a");
  a.setAttribute("href", tab.url);
  a.innerText = link;
  div.appendChild(a);
  if (description) {
    let text = document.createTextNode(" - " + description);
    div.appendChild(text);
  }

  // Hide container.
  div.classList.add("container");

  // Add container to popup.
  document.body.appendChild(div);
  
  // Copy it.
  window.getSelection().removeAllRanges();
  let range = document.createRange();
  range.selectNode(div);
  window.getSelection().addRange(range);
  document.execCommand("copy");
  window.getSelection().removeAllRanges();

  // Remove container.
  document.body.removeChild(div);

  // Tell the user we did something.
  document.body.innerText = "Copied!"
}

function githubMatcher(tab) {
  let m = tab.url.match(/github\.com\/golang\/go\/issues\/(\d+)/);
  if (!m) {
    return null;
  }
  let i = tab.title.indexOf(" ·");
  return {
    link: "#" + m[1],
    description: i < 0 ? tab.title : tab.title.slice(0, i)
  }
}

function gerritMatcher(tab) {
  let m = tab.url.match(/go-review\.googlesource\.com\/c\/go\/\+\/(\d+)/);
  if (!m) {
    return null;
  }
  let link = "CL " + m[1];
  let description = "";
  m = tab.title.match(/(.*) \(I[0-9a-f]+\) · Gerrit Code Review/);
  if (m) {
    description = m[1];
  }
  return {link: link, description: description};
}

window.onload = noteLink;

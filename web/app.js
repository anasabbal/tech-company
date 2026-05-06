const state = {
  jobs: [],
  countries: [],
  stats: {},
};

const elements = {
  statGrid: document.getElementById("statGrid"),
  updatedAt: document.getElementById("updatedAt"),
  searchInput: document.getElementById("searchInput"),
  countryFilter: document.getElementById("countryFilter"),
  sortFilter: document.getElementById("sortFilter"),
  clearFilters: document.getElementById("clearFilters"),
  resultsTitle: document.getElementById("resultsTitle"),
  jobsGrid: document.getElementById("jobsGrid"),
  emptyState: document.getElementById("emptyState"),
  cardTemplate: document.getElementById("jobCardTemplate"),
};

const addedDate = (value) => {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? new Date(0) : parsed;
};

const formatNumber = (value) => new Intl.NumberFormat().format(value);

const renderStats = () => {
  const labels = [
    ["jobs", "Open jobs"],
    ["companies", "Companies"],
    ["countries", "Countries"],
  ];

  elements.statGrid.innerHTML = "";
  labels.forEach(([key, label]) => {
    const item = document.createElement("div");
    item.className = "stat-card";
    item.innerHTML = `<strong>${formatNumber(state.stats[key] || 0)}</strong><span>${label}</span>`;
    elements.statGrid.appendChild(item);
  });
};

const populateCountries = () => {
  state.countries.forEach((country) => {
    const option = document.createElement("option");
    option.value = country;
    option.textContent = country;
    elements.countryFilter.appendChild(option);
  });
};

const filterJobs = () => {
  const search = elements.searchInput.value.trim().toLowerCase();
  const country = elements.countryFilter.value;
  const sort = elements.sortFilter.value;

  let jobs = state.jobs.filter((job) => {
    const matchesSearch =
      search === "" ||
      [job.company, job.job_title, job.location, job.country]
        .join(" ")
        .toLowerCase()
        .includes(search);

    const matchesCountry = country === "all" || job.country === country;
    return matchesSearch && matchesCountry;
  });

  jobs.sort((a, b) => {
    if (sort === "company") {
      return a.company.localeCompare(b.company) || a.job_title.localeCompare(b.job_title);
    }
    if (sort === "title") {
      return a.job_title.localeCompare(b.job_title) || a.company.localeCompare(b.company);
    }
    return addedDate(b.added) - addedDate(a.added);
  });

  return jobs;
};

const renderJobs = () => {
  const jobs = filterJobs();
  elements.jobsGrid.innerHTML = "";

  elements.resultsTitle.textContent = `${formatNumber(jobs.length)} roles ready to explore`;
  elements.emptyState.classList.toggle("hidden", jobs.length > 0);

  jobs.forEach((job) => {
    const fragment = elements.cardTemplate.content.cloneNode(true);
    fragment.querySelector(".company-line").textContent = job.company;
    fragment.querySelector(".job-title").textContent = job.job_title;
    fragment.querySelector(".country-pill").textContent = `${job.country_flag} ${job.country}`;
    fragment.querySelector(".location").textContent = `Location: ${job.location}`;
    fragment.querySelector(".added").textContent = `Added: ${job.added}`;

    const link = fragment.querySelector(".apply-link");
    link.href = job.apply_url;
    link.setAttribute("aria-label", `Apply for ${job.job_title} at ${job.company}`);

    elements.jobsGrid.appendChild(fragment);
  });
};

const setUpdatedAt = (value) => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    elements.updatedAt.textContent = "";
    return;
  }

  elements.updatedAt.textContent = `Loaded ${date.toLocaleString([], {
    dateStyle: "medium",
    timeStyle: "short",
  })}`;
};

const resetFilters = () => {
  elements.searchInput.value = "";
  elements.countryFilter.value = "all";
  elements.sortFilter.value = "recent";
  renderJobs();
};

const wireEvents = () => {
  elements.searchInput.addEventListener("input", renderJobs);
  elements.countryFilter.addEventListener("change", renderJobs);
  elements.sortFilter.addEventListener("change", renderJobs);
  elements.clearFilters.addEventListener("click", resetFilters);
};

const init = async () => {
  const response = await fetch("/api/jobs");
  const payload = await response.json();

  state.jobs = payload.jobs || [];
  state.countries = payload.countries || [];
  state.stats = payload.stats || {};

  renderStats();
  populateCountries();
  setUpdatedAt(payload.last_updated_iso);
  wireEvents();
  renderJobs();
};

init().catch((error) => {
  console.error(error);
  elements.resultsTitle.textContent = "Could not load jobs";
  elements.emptyState.classList.remove("hidden");
  elements.emptyState.innerHTML =
    "<h3>Something went wrong.</h3><p>Please restart the server and try again.</p>";
});

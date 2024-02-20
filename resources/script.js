(() => {
	class ComponentSet {
		constructor(kind) {
			this.kind = kind;
			let components = {};
			const root = document.querySelector(`[data-server-kind="${kind}"]`);
			if (!root) {
				this.ok = false;
				return;
			}

			components.root = root;
			components.incarnationCounter = root.querySelector(".incarnation");
			components.periodCounter = root.querySelector(".period");
			components.membersTable = root.querySelector(".members-table");
			components.membersBody = components.membersTable.querySelector("tbody");
			components.currentGossipsTable = root.querySelector(".gossips-table");
			components.currentGossipsBody = components.currentGossipsTable.querySelector("tbody");
			components.allGossipsTable = root.querySelector(".gossips table");
			components.allGossipsBody = components.allGossipsTable.querySelector("tbody");
			components.clearButton = root.querySelector(".gossips a");
			components.clearButton.addEventListener("click", (e) => {
				e.preventDefault();
				e.stopPropagation();
				e.stopImmediatePropagation();

				this.components.allGossipsBody.replaceChildren(this.makeGossipWaitingForData());
			});

			this.components = components;
			this.ok = true;
			this.isGossipsEmpty = false;
			this.isMembersEmpty = false;
		}

		setPeriod(p) { this.components.periodCounter.innerHTML = (p || 0).toString(); }
		setIncarnation(i) { this.components.incarnationCounter.innerHTML = (i || 0).toString(); }

		setCurrentGossips(list) {
			list = list || [];
			if (list.length === 0 && this.isGossipsEmpty) {
				return;
			}
			if (this.isGossipsEmpty) {
				this.components.currentGossipsBody.firstChild.remove();
				this.isGossipsEmpty = false;
			}

			this.components.currentGossipsBody.querySelectorAll(".faded").forEach(el => el.remove());
			Array.from(this.components.currentGossipsBody.children).forEach(el => el.classList.add("faded"));
			list
				.map(i => this.makeCurrentGossipItem(i))
				.forEach(i => this.components.currentGossipsBody.prepend(i));

			if (this.components.currentGossipsBody.children.length === 0) {
				this.components.currentGossipsBody.append(this.makeEmptyCurrentGossipItem());
				this.isGossipsEmpty = true;
			}
		}

		setKnownMembers(list) {
			list = list || [];
			if (list.length === 0 && this.isMembersEmpty) {
				return;
			}

			if (this.isMembersEmpty) {
				this.components.membersBody.firstChild.remove();
				this.isMembersEmpty = false;
			}

			this.components.membersBody
				.replaceChildren(...list.map(i => this.makeKnownMember(i)));

			if (this.components.membersBody.children.length === 0) {
				this.components.membersBody.append(this.makeEmptyKnownMember());
				this.isMembersEmpty = true;
			}
		}

		appendGossip(gossip) {
			this.components.allGossipsBody.querySelectorAll(".empty").forEach(i => i.remove());
			this.components.allGossipsBody.prepend(this.makeGossipItem(gossip));
		}

		makeTableDataCell(count) {
			return Array.from(new Array(count), () => document.createElement("td"));
		}

		makeCurrentGossipItem(gossip) {
			const children = this.makeTableDataCell(4);
			children[0].classList.add("mono");
			children[0].innerText = gossip.source || "";

			children[1].append(this.makeLabel(gossip.kind));

			children[2].classList.add("mono");
			children[2].innerText = gossip.subject || "";

			children[3].classList.add("mono");
			children[3].innerText = gossip.incarnation || "";

			const tr = document.createElement("tr");
			children.forEach(c => tr.append(c));
			return tr;
		}

		makeEmptyCurrentGossipItem() {
			const children = this.makeTableDataCell(4);
			children[0].innerText = "No gossips";

			const tr = document.createElement("tr");
			children.forEach(c => tr.append(c));
			return tr;
		}

		makeLabel(value) {
			const span = document.createElement("span");
			span.classList.add("label", value);
			span.innerText = value;
			return span;
		}

		makeKnownMember(member) {
			const children = this.makeTableDataCell(3);
			children[0].classList.add("mono");
			children[0].innerText = member.ip;
			children[1].append(this.makeLabel(member.status));
			children[2].classList.add("mono");
			children[2].innerText = (member.incarnation || 0).toString();

			const tr = document.createElement("tr");
			children.forEach(i => tr.append(i));
			return tr;
		}

		makeEmptyKnownMember() {
			const children = this.makeTableDataCell(3);
			children[0].innerText = "No known members";

			const tr = document.createElement("tr");
			children.forEach(c => tr.append(c));
			return tr;
		}

		makeGossipItem(gossip) {
			const children = this.makeTableDataCell(6);
			children.forEach(i => i.classList.add("mono"));
			children[0].innerText = gossip.direction;
			children[1].innerText = gossip.timestamp;
			children[2].innerText = gossip.source;
			children[3].append(this.makeLabel(gossip.kind));
			children[4].innerText = gossip.subject;
			children[5].innerText = (gossip.incarnation || 0).toString();

			const tr = document.createElement("tr");
			children.forEach(i => tr.append(i));
			return tr;
		}

		makeGossipWaitingForData() {
			const children = this.makeTableDataCell(6);
			children[1].innerText = "Waiting for data";
			children[1].classList.add("mono");

			const tr = document.createElement("tr");
			children.forEach(i => tr.append(i));
			return tr;
		}
	}

	class Worker {
		constructor() {
			const wsURL = new URL("/ws", document.baseURI);
			wsURL.protocol = "ws:";
			this.socket = new WebSocket(wsURL.toString());
			this.socket.addEventListener("open", () => this.socketOpen());
			this.components = {};
			this.initializeDOM();
			this.socket.addEventListener("message", (msg) => {
				let json;
				try {
					json = JSON.parse(msg.data);
				} catch (ex) {
					console.error("Failed reading message from server", msg.data, ex);
					return;
				}
				this.handleEvent(json);
			});
			// TODO: Disconnect/Error event handler
		}

		initializeDOM() {
			let v4 = new ComponentSet("v4");
			if (!v4.ok) v4 = null;
			this.components["v4"] = v4;

			let v6 = new ComponentSet("v6");
			if (!v6.ok) v6 = null;
			this.components["v6"] = v6;
		}

		socketOpen() {
			console.log("connected");
		}

		handleGeneralMessage(msg) {
			const kind = msg.serverKind;
			const data = msg.payload;
			const comp = this.components[msg.serverKind];
			if (!comp) return;

			comp.setCurrentGossips(data.gossips);
			comp.setKnownMembers(data.members);
			comp.setIncarnation(data.incarnation);
			comp.setPeriod(data.period);
		}

		handleGossip(msg) {
			const kind = msg.serverKind;
			const comp = this.components[msg.serverKind];
			if (!comp) return;

			comp.appendGossip(msg.payload);
		}

		handleEvent(event) {
			switch (event.kind) {
				case "general":
					return this.handleGeneralMessage(event);
				case "gossip":
					return this.handleGossip(event);
				default:
					console.warn("Received unknown event from server", event);
					return;
			}
		}
	}

	document.worker = new Worker();
})();

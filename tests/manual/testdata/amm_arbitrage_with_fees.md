# Automated Market Making and Arbitrage Profits in the Presence of Fees

Jason Milionis Department of Computer Science Columbia University jm@cs.columbia.edu

Ciamac C. Moallemi   
Graduate School of Business Columbia University   
ciamac@gsb.columbia.edu   
Tim Roughgarden   
Department of Computer Science   
Columbia University   
a16z Crypto   
tim.roughgarden@gmail.com

Initial version: February 6, 2023   
Current version: July 23, 2025

## Abstract

We consider the impact of trading fees on the profits of arbitrageurs trading against an automated market maker (AMM) or, equivalently, on the adverse selection incurred by liquidity providers (LPs) due to arbitrage. We extend the model of Milionis et al. [2022] for a general class of two asset AMMs to introduce both fees and discrete Poisson block generation times. In our setting, we are able to compute the expected instantaneous rate of arbitrage profit in closed form. When the fees are low, in the fast block asymptotic regime, the impact of fees takes a particularly simple form: fees simply scale down arbitrage profits by the fraction of blocks which present profitable trading opportunities to arbitrageurs. This fraction decreases with an increasing block rate, hence our model yields an important practical insight: faster blockchains will result in reduced LP losses. Further introducing gas fees (fixed costs) in our model, we show that, in the fast block asymptotic regime, lower gas fees lead to smaller losses for LPs.

## 1. Introduction

For automated market makers (AMMs), the primary cost incurred by liquidity providers (LPs) is adverse selection. Adverse selection arises from the fact that agents (“arbitrageurs”) with an informational advantage, in the form of knowledge of current market prices, can exploit stale prices on the AMM versus prices on other markets such as centralized exchanges. Because trades between arbitrageurs and the AMM are zero sum, any arbitrage profits will be realized as losses to the AMM LPs. Milionis et al. [2022] quantify these costs through a metric called loss-versusrebalancing (LVR). They establish that LVR can be simultaneously interpreted as: (1) arbitrage profits due to stale AMM prices; (2) the loss incurred by LPs relative to a trading strategy (the “rebalancing strategy”) that holds the same risky positions as the pool, but that trades at market prices rather than AMM prices; and (3) the value of the lost optionality when an LP commits upfront to a particular liquidity demand curve. They develop formulas for LVR in closed form, and show theoretically and empirically that, once market risk is hedged, the profit-and-loss (P&L) of an LP reduces to trading fee income minus LVR. In this way, LVR isolates the costs of liquidity provision.

Despite its benefits, LVR suffers from a significant flaw: it is derived under the simplification that arbitrageurs do not pay trading fees. In practice, however, trading fees pose a significant friction and limit arbitrage profits. The main contribution of the present work is to develop a tractable model for arbitrage profits in the presence of trading fees. We are able to obtain general formulas for arbitrageur profits in this setting. We establish that arbitrage profits in the presence of fees are roughly equivalent to the arbitrage profits in the frictionless case (i.e., LVR), but scaled down to adjust for the fraction of time where the AMM price differs from the market price significantly enough that arbitrageurs can make profits even in the presence of fees. That is, the introduction of fees can be viewed as a rescaling of time.

Our goal is to introduce fees and understand how they impact arbitrageur behavior. As a starting point, one could directly introduce fees into the model of Milionis et al. [2022], where prices follow a geometric Brownian motion and arbitrageurs continuously monitor the AMM. However, this approach suffers a major pathology: when arbitrageurs monitor the market continuously in the presence of even negligible non-zero fees, the arbitrage profits are zero! Intuitively, when there are no fees, every instantaneous price movement provides a profitable arbitrage opportunity. With fees, this is true only for movements outside a (fee-dependent) “no-trade region” around the AMM price which, with continuous monitoring, then results in an immediate repositioning of that region. One can show that the fraction of time for which this happens is zero, with the market price inside the no-trade region at all other times. This is analogous to the fact that, in continuous time, a reflected random walk spends almost none of its time at the boundaries. In reality, however, arbitrageurs cannot continuously monitor and trade against the AMM. For example, for an AMM implemented on a blockchain, the arbitrageurs can only act at the discrete times at which blocks are generated. Thus, in order to understand arbitrage profits in the presence of fees, it is critical to model the discreteness of block generation.

## 1.1. Model

Our starting point is the model of Milionis et al. [2022], where arbitrageurs continuously monitor an AMM to trade a risky asset versus the numéraire, and the risky asset price follows geometric Brownian motion parameterized by volatility $\sigma > 0$ . However, we assume that the AMM has a trading fee $\gamma \geq 0 ,$ and that arbitrageurs arrive to trade on the AMM at discrete times according to the arrivals of a Poisson process with rate $\lambda > 0$ . The Poisson process is a natural choice because of its memoryless nature and standard usage throughout continuous time finance. It is natural to assume arrival times correspond to block generation times, since the arbitrageurs can only trade at instances at which a block is generated, so the parameter λ should be calibrated so that the mean interarrival time $\Delta t \triangleq \lambda ^ { - 1 }$ corresponds to the mean interblock time.

When an arbitrageur arrives, they seek to make a trade that myopically maximizes their immediate profit. Arbitrageurs trade myopically because of competition. If they choose to forgo immediate profit but instead wait for a larger mispricing, they risk losing the profitable trading opportunity to the next arbitrageur. If the AMM price net of fees is below (respectively, above) the market price, the arbitrageur will buy (sell) from the pool and sell (buy) at the market. They will do so until the net marginal price of the AMM equals the market price. We describe these dynamics in terms of a mispricing process that is the difference between the AMM and market log-prices. At each arrival time, a myopic arbitrageur will trade in a way such that the pool mispricing to jumps to the nearest point in band. The width of the band is determined by the fee γ. We call this band the no-trade region, since if the arbitrageur arrives and the mispricing is already in the band, there is no profitable trade possible. At all non-arrival times, the mispricing is a diffusion, driven by the geometric Brownian motion governing market prices.

## 1.2. Results

In our setting, the mispricing process is a Markovian jump-diffusion process. Our first result (Theorem 1) is to establish that this process is ergodic, and to identify its steady state distribution in closed form. Under this distribution, the probability that, at the instance a block is generated, an arbitrageur can make profitable trade, i.e., the fraction of time that the mispricing process is outside the no-trade region in steady state, is given by

$$
\mathsf { P } _ { \mathsf { t r a d e } } \triangleq \frac { 1 } { 1 + \underbrace { \sqrt { 2 \lambda } \gamma / \sigma } _ { \triangleq \eta } . }
$$

This can also be interpreted as the long run fraction of blocks that contain an arbitrage trade. $\mathsf { P _ { t r a d e } }$ has intuitive structure in that it is a function of the composite parameter $\eta \triangleq \gamma / ( \sigma \sqrt { \lambda ^ { - 1 } / 2 } )$ ， the fee measured as a multiple of the typical (one standard deviation) movement of returns over half the average interarrival time. When η is large (e.g., high fee, low volatility, or frequent blocks), the width of the no-fee region is large relative to typical interarrival price moves, so the mispricing process is less likely to exit the no-trade region in between arrivals, and $\mathsf { P _ { t r a d e } } \approx \eta ^ { - 1 }$

Given the steady state distribution of the pool mispricing, we can quantify the arbitrage profits. Denote by $\mathsf { A R B } _ { T }$ the cumulative arbitrage profits over the time interval [0, T ]. We compute the expected instantaneous rate of arbitrage profit $\overline { { \mathsf { A R B } } } \triangleq \mathrm { l i m } _ { T  0 } \mathsf { E } [ \mathsf { A R B } _ { T } ] / T$ , where the expectation is over the steady state distribution of mispricing. We derive a semi-closed form expression (involving an expectation) for ARB (Theorem 2). For specific cases, such as geometric mean or constant product market makers, this expectation can be evaluated resulting in an explicit closed form (Corollary 2).

We further consider an asymptotic analysis in the fast block regime where $\lambda \to \infty$ (Theorem 3). Equivalently, this is the limit as the mean interblock time $\Delta t \triangleq \lambda ^ { - 1 } \to 0 )$ . In order to explain our asymptotic results, we begin with the frictionless base case of Milionis et al. [2022], where there is no fee $( \gamma = 0 )$ and continuous monitoring $( \lambda = \infty )$ . Milionis et al. [2022] establish that the expected instantaneous rate of arbitrage profit is

$$
\overline { { \mathsf { L V R } } } \triangleq \operatorname* { l i m } _ { T  0 } \frac { \mathsf { E } [ \mathsf { L V R } _ { T } ] } { T } = \frac { \sigma ^ { 2 } P } { 2 } \times y ^ { * \prime } ( P ) .\tag{1}
$$

Here, $P$ is the current market price, while $y ^ { * } ( P )$ is the quantity of numéraire held by the pool when the market price is $P ,$ so that $y ^ { * } { } ^ { \prime } ( P )$ is the marginal liquidity of the pool at price P , denominated in the numéraire. In the presence of fees and discrete monitoring, our rigorous analysis establishes that as $\lambda \to \infty$

$$
\overline { { \mathsf { A R B } } } \triangleq \operatorname* { l i m } _ { T  0 } \frac { \mathsf { E } [ \mathsf { A R B } _ { T } ] } { T } = \frac { \sigma ^ { 2 } P } { 2 } \times \underbrace { \frac { y ^ { \ast \prime } ( P e ^ { - \gamma } ) + e ^ { + \gamma } \cdot y ^ { \ast \prime } ( P e ^ { + \gamma } ) } { 2 } } _ { = y ^ { \ast \prime } ( P ) + O ( \gamma ) \mathrm { f o r ~ } \gamma \mathrm { s m a l l } } \times \underbrace { \frac { 1 } { 1 + \sqrt { 2 \lambda } \gamma / \sigma } } _ { = \mathsf { P r a d e } } + o ( \sqrt { \lambda ^ { - 1 } } ) .\tag{2}
$$

Equations (1) and (2) differ in two ways. First, (1) involves the marginal liquidity $y ^ { * } { } ^ { \prime } ( P )$ at the current price P , while (2) averages the marginal liquidity at the endpoints of the no-trade interval of prices $[ P e ^ { - \gamma } , P e ^ { + \gamma } ]$ . This difference is minor if the fee $\gamma$ is small. The second difference, which is major, is that arbitrage profits in (2) are scaled down relative to (1) by precisely the factor $\mathsf { P _ { t r a d e } }$ . In other words, if the fee is low, in the fast block regime we can view the impact of the fee on arbitrage profits as scaling down LVR by the fraction of time that an arriving arbitrageur can profitably trade: $\overline { { \mathsf { A R B } } } \approx \overline { { \mathsf { L V R } } } \times \mathsf { P } _ { \mathsf { t r a d e } }$

Focusing on the dependence on problem parameters, when $\gamma > 0 ,$ (2) implies that in the fast block regime arbitrage profits are proportional to the square root of the mean interblock time $( { \sqrt { \lambda ^ { - 1 } } } )$ , the cube of the volatility $( \sigma ^ { 3 } )$ , and the reciprocal of the fee $( \gamma ^ { - 1 } )$ . These scaling dependencies are consistent with the results of Nezlobin and Tassy [2025], who consider a similar problem with a stylized AMM and general block-time distributions. Equation (2) also highlights an interesting phase transition with the introduction of fees. Specifically, in the absence of fees $( \gamma = 0 )$ , in the fast block regime $( \lambda \to \infty )$ , we have the $\overline { { \mathsf { A R B } } } = \overline { { \mathsf { L V R } } } + o ( 1 ) = \Theta ( 1 )$ , i.e., up to a first order, arbitrage profits per unit time are constant and do not depend on the interblock time. On the other hand, when there are fees $( \gamma > 0 )$ , we have that ${ \overline { { \mathsf { A R B } } } } = \Theta ( { \sqrt { \lambda ^ { - 1 } } } )$ , arbitrage profits per unit time scale with the square root of the interblock time. In other words, our model yields an important practical insight: that LP losses to arbitrageurs are reduced on faster blockchains.

Considering the fees paid by arbitrageurs to the pool, define FEE to be the instantaneous rate of arbitrage fees. We establish that (Theorem 4), asympotitically, FEE $\approx \overline { { \mathsf { L V R } } } \times ( 1 - \mathsf { P } _ { \mathsf { t r a d e } } )$ when the fee $\gamma$ is small in the fast block regime. This implies that, assuming fee $\gamma$ is small and we are in the fast block regime, ARB + FEE ≈ LVR, which can be interpreted as LVR being split among fees and arbitrage profits, according to $\mathsf { P _ { t r a d e } }$ . In particular, as the blocks become more and more frequent (for a fixed fee γ), LVR is redirected from arbitrage profits to fees paid by arbitrageurs.

Finally, we construct a model as above with the addition of gas fees (Section 6), i.e., fixed transaction costs associated with executing any potential arbitrage transaction. Our results in the fast block regime show that lower gas fees result in smaller losses to LPs. We also establish in this model that, when both fixed (gas) and marginal (trading) fees are small, all of these LP losses leak to the validators in gas fees, elucidating that they are the true recipients of the informational losses due to the stale prices of AMMs.

## 1.3. Conclusion

This work has broad implications around liquidity provision and the design of automated market makers:

• Blockchain architecture implications: The asymptotic regime analysis λ → ∞ above points to a significant potential mitigator of arbitrage profits: running a chain with lower mean interblock time $\Delta t \triangleq \lambda ^ { - 1 }$ (essentially, a faster chain), since we show that this effectively reduces arbitrage profit without negatively impacting LP fee income derived from noise trading. Similarly, reduction of gas costs reduces arbitrage profits. We discuss this in Section 7.1.

• Pricing accuracy: Setting a low fee enables accurate prices, since arbitrageurs can then correct even small discrepancies, but this comes at the cost of higher arbitrage profits. Our model can crisply characterize this tradeoff. We discuss this in Section 7.2.

• Improved LP performance modeling. Our model provides a more accurate quantification of LP P&L, accounting both for arbitrageurs paying trading fees and discrete arbitrageur arrival times. Our results thus have the potential to better inform AMM design, and in particular, provide guidance around how to set trading fees in an AMM to balance LP fee income from noise traders and LP loss due to arbitrageurs. Our results can also be used to contruct equilibria for LPs in counterfactual settings. We discuss this in Section 7.3.

These findings provide a comprehensive framework for understanding and optimizing automated market maker performance in the presence of realistic market frictions.

## 1.4. Literature Review

There is a rich literature on automated market makers. Angeris and Chitra [2020] and Angeris et al. [2021a,b] apply tools from convex analysis (e.g., the pool reserve value function) that we also use in this paper. In the first paper to decompose the return of an LP into an instantaneous market risk component and a non-negative, non-decreasing, and predictable component called “loss-versus-rebalancing” (LVR, pronounced “lever”), Milionis et al. [2022] analyze the frictionless, continuous-time Black-Scholes setting in the absence of trading fees to show that it is exactly the adverse selection cost due to the arbitrageurs’ informational advantage to the pool. This work extends the model of Milionis et al. [2022] to account for arbitrage profits both in the presence of fees and discrete-time arbitrageur arrivals. Broader classes of AMMs that have locally smooth demand curves but are not necessarily constant function market makers have been given by Milionis et al. [2023, 2024]; our model here applies to such a general case as well. Evans et al. [2021] observe that, in the special case of geometric mean market makers, taking the limit to continuous time while holding the fees $\gamma > 0$ fixed and strictly positive yields vanishing arbitrage profits; this is also a special case of our results. Angeris et al. [2021b] also analyze arbitrage profits, but do not otherwise express them in closed-form. Black-Scholes-style options pricing models, like the ones developed in this paper, have been applied to weighted geometric mean market makers over a finite time horizon by Evans [2020], who also observes that constant product pool values are a supermartingale because of negative convexity. Clark [2020] replicates the payoff of a constant product market over a finite time horizon in terms of a static portfolio of European put and call options. Tassy and White [2020] compute the growth rate of a constant product market maker with fees. Dewey and Newbold [2023] develop a model of pricing and hedging AMMs with arbitrageurs and noise traders and conjecture that arbitrageurs induce the same stationary distribution of mispricing that we rigorously develop here.

Since it first appeared, our model has been influential in the broader discussion of Maximal Extracted Value (MEV) [Daian et al., 2020]. Arbitrage profit in our setting models real world CEX-DEX arbitrage profits, which are thought to be the dominant form of MEV. Reducing this MEV has been an important goal for practitioners, and our work has been cited by practitioners as a motivation to seek smaller block times in order to reduce MEV. Recent empirical work by Fritsch and Canidio [2024] (discussed in Section 7.1) provides strong validation of our theoretical predictions regarding the relationship between block times and arbitrage profits.

Subsequent to the initial publication of this work, Nezlobin and Tassy [2025] consider a setting similar to ours, and the main innovation of their important work is to propose an alternative methodology which allows for more general block-time distributions. They study a specific, stylized AMM where the arbitrageur needs to trade $\ell \times \Delta$ in numéraire value to move the quoted price by ∆ units of return, where ℓ is a constant marginal liquidity parameter. In this setting, they can asymptotically compute the intensity of arbitrage profits ARB, and derive a general decomposition of the form $\overline { { \mathsf { A R B } } } \approx \mathsf { P } _ { \mathrm { t r a d e } } \times \overline { { \mathsf { L V R } } }$ , as we do here. In the case of a Poisson block-generation process, their results recover and validate our original results (using a different technical methodology).1 However, their results are more general than ours in that they can handle arbitrary block-time distributions. Indeed, they establish the striking result that ARB is minimized (among all blocktime distributions with fixed mean) by the deterministic block arrival process. That said, their methodology has some limitations with respect to our methodology. Our results are more general in that they are applicable to all AMMs that satisfy very mild technical conditions, and are not restricted to a stylized constant marginal liquidity AMM. We also derive closed form as well as asymptotic expressions, are able to derive the fees obtained by the AMM, and are able to handle gas fees.

## 2. Model

Assets. Fix a filtered probability space $\left( \Omega , \mathcal { F } , \{ \mathcal { F } _ { t } \} _ { t \ge 0 } \right)$ satisfying the usual assumptions. Consider two assets: a risky asset x and a numéraire asset y. Working over continuous times $t \in \mathbb { R } _ { + }$ , assume that there is observable external market price $P _ { t }$ at each time t. The price $P _ { t }$ evolves exogenously according to the geometric Brownian motion

$$
\frac { d P _ { t } } { P _ { t } } = \mu d t + \sigma d B _ { t } , \quad \forall t \geq 0 ,
$$

with drift $\mu ,$ volatility $\sigma > 0$ , and where $B _ { t }$ is a Brownian motion.

AMM Pool. We assume that the AMM operates as a constant function market maker (CFMM).2 The state of a CFMM pool is characterized by the reserves $( x , y ) \in \mathbb { R } _ { + } ^ { 2 }$ , which describe the current holdings of the pool in terms of the risky asset and the numéraire, respectively. Define the feasible set of reserves C according to

$$
\mathcal { C } \triangleq \{ ( x , y ) \in \mathbb { R } _ { + } ^ { 2 } \ : \ f ( x , y ) = L \} ,
$$

where $f \colon  { \mathbb { R } } _ { + } ^ { 2 } \to  { \mathbb { R } }$ is referred to as the bonding function or invariant, and $L \in \mathbb { R }$ is a constant.3 In other words, the feasible set is a level set of the bonding function. The pool is defined by a smart contract which allows an agent to transition the pool reserves from the current state $( x _ { 0 } , y _ { 0 } ) \in \mathcal { C }$ to any other point $( x _ { 1 } , y _ { 1 } ) \in { \mathcal { C } }$ in the feasible set, so long as the agent contributes the difference $( x _ { 1 } - x _ { 0 } , y _ { 1 } - y _ { 0 } )$ into the pool, see Figure 1a.

Define the pool value function $V : \mathbb { R } _ { + } \to \mathbb { R } _ { + }$ by the optimization problem

$$
V ( P ) \triangleq { \begin{array} { l l } { \operatorname { m i n i m i z e } } & { P x + y } \\ { ( x , y ) \in \mathbb { R } _ { + } ^ { 2 } } \\ { \operatorname { s u b j e c t } \operatorname { t o } } & { f ( x , y ) = L . } \end{array} }\tag{3}
$$

The pool value function yields the value of the pool, assuming that the external market price of the risky asset is given by P , and that arbitrageurs can instantaneously trade against the pool maximizing their profits (and simultaneously minimizing the value of the pool). Geometrically, the pool value function implicitly defines a reparameterization of the pool state from primal coordinates (reserves) to dual coordinates (prices); this is illustrated in Figure 1b.

![](images/a4e2379587c24418a7a7168daa81c6dd59820c8c0fa6ed7bbd4218d96272f000.jpg)  
(a) Transitions between any two points on the bonding curve $f ( x , y ) = L$ are permitted, if an agent contributes the difference into the pool.

![](images/9a6e74e82aa78525a5cc2bdbe04ca686eddf9b675b1bd924761cb31dc1121300.jpg)  
(b) The pool value optimization problem relates points on the bonding curve to supporting hyperplanes defined by prices.  
Figure 1: Illustration of a CFMM.

Following Milionis et al. [2022], we assume that the pool value function satisfies:

Assumption 1. (i) An optimal solution $( x ^ { * } ( P ) , y ^ { * } ( P ) )$ to the pool value optimization (3) exists for every $P \geq 0$

(ii) The pool value function $V ( \cdot )$ is everywhere twice continuously differentiable.

(iii) For al l $t \geq 0$

$$
\mathsf { E } \left[ \int _ { 0 } ^ { t } x ^ { * } ( P _ { s } ) ^ { 2 } P _ { s } ^ { 2 } d s \right] < \infty .
$$

We refer to $( x ^ { * } ( P ) , y ^ { * } ( P ) )$ as the demand curves of the pool for the risky asset and numéraire, respectively. Assumption 1(i)–(ii) is a sufficient condition for the following:

Lemma 1. For all prices $P \geq 0$ , the pool value function satisfies:

(i) $V ( P ) \geq 0 .$

(ii) $V ^ { \prime } ( P ) = x ^ { * } ( P ) \geq 0 .$

(iii) $V ^ { \prime \prime } ( P ) = x ^ { * \prime } ( P ) = - P y ^ { * \prime } ( P ) \leq 0 .$

The proof of Lemma 1 follows from standard arguments in convex analysis; see Milionis et al. [2022] for details.

Fee Structure. Suppose that $( \Delta x , \Delta y )$ is a feasible trade permitted by the pool invariant, i.e., given initial pool reserves $( x , y )$ with $f ( x , y ) = L$ , we have $f ( x + \Delta x , y + \Delta y ) = L$ . We assume that an additional proportional trading fee is paid to the LPs in the pool. The mechanics of this trading fee are as follows:

1. The fee is paid in the input asset, i.e., the asset that is contributed to the pool.

2. The fee is realized as a separate cashflow to the $\mathrm { L P s . ^ { 4 } }$

3. We allow for different fees to be paid when the risky asset is bought from the pool and when the risky asset is sold to the pool.

4. We denote the fee in units of log price by $\gamma _ { + } , \gamma _ { - } > 0$ . In particular, when the agent purchases the risky asset from the pool $( \mathrm { i . e . , ~ } \Delta x < 0 , \Delta y > 0 )$ , the total fee charged is

$$
\Big ( e ^ { + \gamma _ { + } } - 1 \Big ) | \Delta y | ,\tag{4}
$$

while the total fee charged when the agent sells the risky asset to the pool $( \mathrm { i . e . , ~ } \Delta x > 0$ $\Delta y < 0 \mathrm { ~ i s ~ } ( e ^ { + \gamma _ { - } } - 1 ) | \Delta x |$ , or, valued in the numéraire at price $P _ { \mathrm { : } }$

$$
P \Big ( e ^ { + \gamma _ { - } } - 1 \Big ) | \Delta x | .\tag{5}
$$

Note that, for notational simplicity, we have chosen to denominate the fee in units of log price. This is mathematically equivalent to standard proportional fees, as illustrated in the following example:

Example 1. In our notation, a 30 basis point proportional fee on either buys or sales $( e . g . ,$ as in Uniswap v2) would correspond to setting $\gamma _ { + } , \gamma _ { - }$ so that

$$
e ^ { + \gamma _ { + } } - 1 = e ^ { + \gamma _ { - } } - 1 = 0 . 0 0 3 ,
$$

so that

$$
\gamma _ { + } = \gamma _ { - } = \log ( 1 + 0 . 0 0 3 ) \approx 0 . 0 0 2 9 9 5 5 0 9 .
$$

To a first order, $\gamma _ { + } = \gamma _ { - } \approx 3 0$ (basis points).

## 3. Arbitrageurs & Pool Dynamics

At any time $t \geq 0$ , define $\tilde { P } _ { t }$ to be the price of the risky asset implied by pool reserves, i.e., the reserves are given by $( x ^ { * } ( \tilde { P } _ { t } ) , y ^ { * } ( \tilde { P } _ { t } ) )$ . Denote by

$$
z _ { t } \triangleq \log P _ { t } / \tilde { P } _ { t } ,\tag{6}
$$

the log mispricing of the pool, so that $\tilde { P } _ { t } = P _ { t } e ^ { - z _ { t } }$

We imagine that arbitrageurs arrive to trade against the pool at discrete times according to a Poisson process of rate $\lambda > 0$ . Here, we imagine that arbitrageurs are continuously monitoring the market, but can only trade against the pool at discrete times when blocks are generated in a blockchain. Hence, we will view the arrival process as both equivalently describing the arrival of arbitrageurs to trade or times of block generation. For a proof-of-work blockchain, Poisson block generation is a natural assumption [Nakamoto, 2008]. However, modern proof-of-state blockchains typically generate blocks at deterministic times. In these cases, we will view the Poisson assumption as an approximation that is necessary for tractability.5 In any case, the mean interarrival time $\Delta t \triangleq \lambda ^ { - 1 }$ should be calibrated to the mean interblock time in a blockchain.

Denote the arbitrageur arrival times (or block generation times) by $0 < \tau _ { 1 } < \tau _ { 2 } < \cdots$ . When an arbitrageur arrives at time $t = \tau _ { i }$ , they can trade against the pool (paying the relevant trading fees) according to the pool mechanism, and simultaneously, frictionlessly trade on an external market at the price $P _ { t }$ . We assume that the arbitrageur will trade to myopically maximize their instantaneous trading profit.6 While we presently ignore any blockchain transaction fees such as $^ { 6 6 } \mathrm { g a s } ^ { 9 9 }$ , we will revisit this in Section 6.

The following lemma (with proof in Appendix A) characterizes the myopic behavior of the arbitrageurs in terms of the demand curves of the pool and the fee structure:

Lemma 2. Suppose that an arbitrageur arrives at time $t = \tau _ { i }$ , observing external market price $P _ { t }$ and implied pool price $\tilde { P } _ { t ^ { - } } \mathrm { ~ } o r ,$ equivalently, mispricing $z _ { t ^ { - } }$ . Then, one of the following three cases applies:

1. If $P _ { t } > \tilde { P } _ { t ^ { - } } e ^ { + \gamma _ { + } }$ or, equivalently, $z _ { t ^ { - } } > + \gamma _ { + }$ , the arbitrageur can profitably buy in the pool and sell on the external market. They will do so until the pool price satisfies $\tilde { P } _ { t } = P _ { t } e ^ { - \gamma _ { + } } ~ o r ,$ equivalently, $z _ { t } = + \gamma _ { + }$ . The arbitrageur profits are then

$$
P _ { t } \left\{ x ^ { * } \left( P _ { t } e ^ { - z _ { t - } } \right) - x ^ { * } \left( P _ { t } e ^ { - \gamma _ { + } } \right) \right\} + e ^ { + \gamma _ { + } } \left\{ y ^ { * } \left( P _ { t } e ^ { - z _ { t - } } \right) - y ^ { * } \left( P _ { t } e ^ { - \gamma _ { + } } \right) \right\} \geq 0 .
$$

2. If $P _ { t } < \tilde { P } _ { t ^ { - } } e ^ { - \gamma _ { - } } o r ,$ equivalently, $z _ { t ^ { - } } < - \gamma _ { - }$ , the arbitrageur can profitably sell in the pool and buy the external market. The will do so until the pool price satisfies $\tilde { P } _ { t } = P _ { t } e ^ { + \gamma _ { - } } o r ,$ equivalently, $z _ { t } = - \gamma .$ −. The arbitrageur profits are then

$$
P _ { t } e ^ { + \gamma _ { - } } \left\{ x ^ { * } \left( P _ { t } e ^ { - z _ { t - } } \right) - x ^ { * } \left( P _ { t } e ^ { + \gamma _ { - } } \right) \right\} + \left\{ y ^ { * } \left( P _ { t } e ^ { - z _ { t - } } \right) - y ^ { * } \left( P _ { t } e ^ { + \gamma _ { - } } \right) \right\} \geq 0 .
$$

3. If $\tilde { P } _ { t ^ { - } } e ^ { - \gamma _ { - } } \leq P _ { t } \leq \tilde { P } _ { t ^ { - } } e ^ { + \gamma _ { + } }$ , or, equivalently, $\begin{array} { r } { \gamma _ { - } \le z _ { t ^ { - } } \le + \gamma _ { + } } \end{array}$ , then the arbitrageur makes no trade, and $\tilde { P } _ { t } = \tilde { P } _ { t ^ { - } } ~ o r$ equivalently, $z _ { t } = z _ { t ^ { - } }$

Considering the three cases in Lemma 2, it is easy to see that, at an arbitrageur arrival time

$\tau _ { i } ,$ the mispricing process $z _ { t }$ evolves according $\mathrm { t o } ^ { 7 }$

$$
z _ { \tau _ { i } } = \mathrm { b o u n d } \{ z _ { \tau _ { i } ^ { - } } , - \gamma _ { - } , + \gamma _ { + } \} .\tag{7}
$$

On the other hand, applying Itô’s lemma to (6), we have that, at other times $t > 0$ , the process evolves according to

$$
\begin{array} { r } { d z _ { t } = \left( \mu - \frac { 1 } { 2 } \sigma ^ { 2 } \right) d t + \sigma d B _ { t } . } \end{array}\tag{8}
$$

Combining (7)–(8), for all $t \geq 0$

$$
z _ { t } = \left( \mu - { \textstyle \frac { 1 } { 2 } } \sigma ^ { 2 } \right) t + \sigma B _ { t } + \sum _ { i : \ \tau _ { i } \leq t } J _ { i } , \qquad J _ { i } \triangleq \mathrm { b o u n d } \left\{ z _ { \tau _ { i } ^ { - } } , - \gamma _ { - } , + \gamma _ { + } \right\} - z _ { \tau _ { i } ^ { - } } .\tag{9}
$$

Therefore, the mispricing process $z _ { t }$ is a Markovian jump-diffusion process. Possible sample paths of these stochastic processes are shown in Figure 2.

![](images/c38d3f525630384ebfdf45e8d37027f797fcafc899bcc1db7cee5969f30448e7.jpg)

![](images/b00583751ac50cd0243797ce13fe2c61852349f0dfe9eecd5a8ec85b07e83598.jpg)  
Figure 2: Top: example sample path of the mispricing process $z _ { t } .$ . Bottom: in red, example external market price process $P _ { t } ;$ in blue, example implied pool price process $\tilde { P } _ { t } .$ − . The no-trade interval is shown in shaded gray; whenever the external market price is within this interval, no trade will happen even if a block is generated. The red- and green-colored crosses in the x-axis show the (Poisson-distributed) times of block generation; red indicates blocks where arbitrageurs do not trade with the pool because the mispricing does not exceed the trading fee, while green indicates blocks where the arbitrageurs do trade. At the green instances, the arbitrageurs trade until the mispricing is equal to the fee and the marginal profit is zero, i.e., the market price is at the edge of the no-trade interval.

## 4. Exact Analysis

We will make the following assumption:

Assumption 2 (Symmetry).

$$
\begin{array} { r } { \mu = \frac 1 2 \sigma ^ { 2 } , \qquad \gamma _ { + } = \gamma _ { - } \triangleq \gamma . } \end{array}
$$

Assumption 2 ensures that the mispricing jump-diffusion process, with dynamics given by (7)– (8), is driftless and has a stationary distribution that is symmetric around $z = 0$ . This assumption will considerably simplify notation and expressions and is without loss of generality. All of our conclusions downstream can be derived without this assumption, at the expense of additional algebra. We discuss this in greater detail in Appendix C, where we also provide a non-symmetric variation of Theorem 1.

## 4.1. Stationary Distribution of the Mispricing Process

The following lemma characterizes the stationary distribution of the mispricing process.8 We defer the proof of this lemma until Appendix B.

Theorem 1 (Stationary Distribution of Mispricing). The process zt is an ergodic process on R, with unique invariant distribution $\pi ( \cdot )$ given by the density

$$
\begin{array} { r } { p _ { \pi } ( z ) = \left\{ \begin{array} { l l } { \pi _ { + } \times p _ { \eta / \gamma } ^ { \mathrm { e x p } } ( z - \gamma ) } & { i f z > + \gamma , } \\ { \pi _ { 0 } \times \frac { 1 } { 2 \gamma } } & { i f z \in [ - \gamma , + \gamma ] , } \\ { \pi _ { - } \times p _ { \eta / \gamma } ^ { \mathrm { e x p } } ( - \gamma - z ) } & { i f z < - \gamma , } \end{array} \right. } \end{array}
$$

$f o r \ z \in \mathbb { R }$ . Here, we define the composite parameter $\eta \triangleq \sqrt { 2 \lambda } \gamma / \sigma$ . The probabilities $\pi _ { - } , \pi _ { 0 } , \pi _ { + }$ of the three segments are given by

$$
\pi _ { 0 } \triangleq \pi \big ( [ - \gamma , + \gamma ] \big ) = \frac { \eta } { 1 + \eta } , \quad \pi _ { + } \triangleq \pi \big ( ( + \gamma , + \infty ) \big ) = \pi _ { - } \triangleq \pi \big ( ( - \infty , - \gamma ) \big ) = \frac { 1 } { 2 } \frac { 1 } { 1 + \eta } .
$$

Final ly, $p _ { \eta / \gamma } ^ { \mathrm { e x p } } ( x ) \triangleq ( \eta / \gamma ) e ^ { - ( \eta / \gamma ) x }$ is the density of an exponential distribution over $x \ge 0$ with parameter $\eta / \gamma = \sqrt { 2 \lambda } / \sigma$

The stationary distribution is illustrated in Figure 3.

<table><tr><td>△t\~</td><td>1bp</td><td>5 bp</td><td>10 bp</td><td>30 bp</td><td>100 bp</td></tr><tr><td>10 min</td><td>96.7%</td><td>85.5%</td><td>74.7%</td><td>49.6%</td><td>22.8%</td></tr><tr><td>2 min</td><td>92.9%</td><td>72.5%</td><td>56.9%</td><td>30.5%</td><td>11.6%</td></tr><tr><td>12 sec</td><td>80.7%</td><td>45.6%</td><td>29.5%</td><td>12.3%</td><td>4.0%</td></tr><tr><td>2 sec</td><td>63.0%</td><td>25.4%</td><td>14.5%</td><td>5.4%</td><td>1.7%</td></tr><tr><td>50 msec</td><td>21.2%</td><td>5.1%</td><td>2.6%</td><td>0.9%</td><td>0.3%</td></tr></table>

Table 1: The probability of trade $\mathsf { P } _ { \sf t r a d e } .$ or, equivalently, the fraction of blocks containing an arbitrage trade, given asset price volatility $\sigma = 5 \%$ (daily), with varying mean interblock times $\Delta t \triangleq \lambda ^ { - 1 }$ and fee levels γ (in basis points).

![](images/9713dca5cc6f51eb0453e02eaf46ffef38513f1e1f5736965bc631a8afe1e4aa.jpg)  
Figure 3: The density $p _ { \pi } ( z )$ of the stationary distribution $\pi ( \cdot )$ of mispricing z, illustrating trade and no-trade regions for an arbitrageur.

Under this distribution, the probability that an arbitrageur arrives and can make a profitable trade, i.e., the fraction of time that the mispricing process is outside the no-trade region in steady state, is given by

$$
\mathsf { P } _ { \mathsf { t r a d e } } \triangleq \pi _ { + } + \pi _ { - } = \frac { 1 } { 1 + \sqrt { 2 \lambda } \gamma / \sigma } .
$$

Equivalently, $\mathsf { P _ { t r a d e } }$ can be interpreted as the long run fraction of blocks that contain an arbitrage trade.

Note that $\mathsf { P _ { t r a d e } }$ does not depend on the bonding function or feasible set defining the CFMM pool; the only pool property relevant is the fee γ. $\mathsf { P _ { t r a d e } }$ has intuitive structure in that it is a function of the composite parameter $\eta \triangleq \gamma / ( \sigma \sqrt { \lambda ^ { - 1 } / 2 } )$ , the fee measured as a multiple of the typical (one standard deviation) movement of returns over half the average interarrival time. When η is large (e.g., high fee, low volatility, or frequent blocks), the width of the no-fee region is large relative to typical interarrival price moves, so the mispricing process is less likely to exit the no-trade region in between arrivals, and $\mathsf { P _ { t r a d e } } \approx \eta ^ { - 1 }$ . Example calculations of $\mathsf { P _ { t r a d e } }$ are shown in Table 1 for $\sigma = 5 \%$ (daily) volatility and varying mean interblock times $\Delta t \triangleq \lambda ^ { - 1 }$ and fee levels $\gamma ,$ as well as in Figure 4a.

![](images/c339c7c278113bff6b2b908628000c8d9dca2ae1c02e26e0b64e78e5b2752b6b.jpg)  
(a) The probability of trade $\mathsf { P _ { t r a d e } }$ , or, equivalently, the fraction of blocks containing an arbitrage trade, as a function of the fee γ.

![](images/6a62cb62adc216b40df81abcc55f85821cd438eafbb049a854070db7170c5878.jpg)  
(b) The standard deviation of mispricing $\sigma _ { z } .$ , as a function of the fee γ.  
Figure 4: Probability of trade and typical mispricing errors as a function of the fee, with $\sigma = 5 \%$ (daily) and mean interblock time $\Delta t \triangleq \lambda ^ { - 1 } = 1 2$ (seconds).

The following immediate corollary quantifies the magnitude of a typical mispricing. This is illustrated in Figure 4b.

Corollary 1 (Standard Deviation of Mispricing). Under the invariant distribution $\pi ( \cdot )$ , the standard deviation of the mispricing is given by

$$
\sigma _ { z } \triangleq { \sqrt { \mathsf { E } _ { \pi } [ z ^ { 2 } ] } } = { \sqrt { ( 1 - \mathsf { P } _ { \mathrm { t r a d e } } ) \times { \frac { 1 } { 3 } } \gamma ^ { 2 } + \mathsf { P } _ { \mathrm { t r a d e } } \times \left\{ \left( \gamma + { \frac { \sigma } { \sqrt { 2 \lambda } } } \right) ^ { 2 } + { \frac { \sigma ^ { 2 } } { 2 \lambda } } \right\} } } .
$$

Note that Figure 4b quantifies the typical mispricing under the invariant distribution $\pi ( \cdot )$ , this is the steady-state distribution that would be observed at the instance of block generation (at the “top-of-the-block”, i.e., before any arbitrage transaction). In the fast block regime $( \lambda \to \infty )$ , we have that

$$
\sigma _ { z } = \frac { \gamma } { \sqrt { 3 } } + O ( \lambda ^ { - 1 / 2 } ) .
$$

In this regime, there is a nonvanishing limit to the mispricing that scales with size of the fee. This is intuitive, as the no-fee band creates a friction that inhibits price corrections.

## 4.2. Rate of Arbitrageur Profit

Denote by $N _ { T }$ the total number of arbitrageur arrivals in [0, T ]. Suppose an arbitrageur arrives at time $\tau _ { i } .$ , observing external price $P _ { \tau _ { i } }$ and mispricing $z _ { \tau _ { i } ^ { - } }$ . From Lemma 2, the arbitrageur profit is

given by

$$
A ( P _ { \tau _ { i } } , z _ { \tau _ { i } ^ { - } } ) \triangleq A _ { + } ( P _ { \tau _ { i } } , z _ { \tau _ { i } ^ { - } } ) + A _ { - } ( P _ { \tau _ { i } } , z _ { \tau _ { i } ^ { - } } ) \geq 0 ,
$$

where we define

$$
A _ { + } ( P , z ) \triangleq \left[ P \left\{ x ^ { * } \left( P e ^ { - z } \right) - x ^ { * } \left( P e ^ { - \gamma } \right) \right\} + e ^ { + \gamma } \left\{ y ^ { * } \left( P e ^ { - z } \right) - y ^ { * } \left( P e ^ { - \gamma } \right) \right\} \right] \mathbb { I } _ { \{ z > + \gamma \} } \geq 0 ,
$$

$$
A _ { - } ( P , z ) \triangleq \left[ e ^ { + \gamma } P \left\{ x ^ { * } \left( P e ^ { - z } \right) - x ^ { * } \left( P e ^ { + \gamma } \right) \right\} + \left\{ y ^ { * } \left( P e ^ { - z } \right) - y ^ { * } \left( P e ^ { + \gamma } \right) \right\} \right] \mathbb { I } _ { \{ z < - \gamma \} } \geq 0 .
$$

Similarly, the fees paid by the arbitrageur in this scenarios are given by

$$
F ( P _ { \tau _ { i } } , z _ { \tau _ { i } ^ { - } } ) \triangleq F _ { + } ( P _ { \tau _ { i } } , z _ { \tau _ { i } ^ { - } } ) + F _ { - } ( P _ { \tau _ { i } } , z _ { \tau _ { i } ^ { - } } ) \geq 0 ,
$$

where we define

$$
F _ { + } ( P , z ) \triangleq - \left( e ^ { + \gamma } - 1 \right) \left[ y ^ { * } \left( P e ^ { - z } \right) - y ^ { * } \left( P e ^ { - \gamma } \right) \right] \mathbb { I } _ { \{ z > + \gamma \} } \geq 0 ,
$$

$$
F _ { - } ( P , z ) \triangleq - \left( e ^ { + \gamma } - 1 \right) P \left[ x ^ { * } \left( P e ^ { - z } \right) - x ^ { * } \left( P e ^ { + \gamma } \right) \right] \mathbb { I } _ { \{ z < - \gamma \} } \geq 0 .
$$

We can write the total arbitrage profit and fees paid over [0, T ] by summing over all arbitrageurs arriving in that interval, i.e.,

$$
\mathsf { A R B } _ { T } \triangleq \sum _ { i = 1 } ^ { N _ { T } } A ( P _ { \tau _ { i } } , \boldsymbol { z } _ { \tau _ { i } ^ { - } } ) , \quad \mathsf { F E E } _ { T } \triangleq \sum _ { i = 1 } ^ { N _ { T } } F ( P _ { \tau _ { i } } , \boldsymbol { z } _ { \tau _ { i } ^ { - } } ) .
$$

Clearly these are non-negative and monotonically increasing processes. The following theorem characterizes their instantaneous expected rate of growth or intensity:9

Theorem 2 (Rate of Arbitrage Profit and Fees). Define the intensity, or instantaneous rate of arbitrage profit, by

$$
{ \overline { { \mathsf { A R B } } } } \triangleq \operatorname* { l i m } _ { T \to 0 } { \frac { \mathsf { E } [ \mathsf { A R B } _ { T } ] } { T } } .
$$

Given initial price $P _ { 0 } = P ,$ , suppose that $z _ { 0 - } = z$ is distributed according to its stationary distribution $\pi ( \cdot )$ . Then, the instantaneous rate of arbitrage profit is given by

$$
\overline { { \mathsf { A R B } } } = \lambda \mathsf { E } _ { \pi } \left[ A ( P , z ) \right] = \lambda \mathsf { P } _ { \mathrm { t r a d e } } \frac { \sqrt { 2 \lambda } } { \sigma } \int _ { 0 } ^ { \infty } \frac { A _ { + } ( P , x + \gamma ) + A _ { - } ( P , - x - \gamma ) } { 2 } e ^ { - \sqrt { 2 \lambda } x / \sigma } d x .\tag{10}
$$

Similarly, defining the intensity of the fee process by

$$
{ \overline { { \mathsf { F E E } } } } \triangleq \operatorname* { l i m } _ { T \to 0 } { \frac { \mathsf { E } \left[ { \mathsf { F E E } } _ { T } \right] } { T } } ,
$$

we have that

$$
\overline { { \mathsf { F E } } } = \lambda \mathsf { E } _ { \pi } \left[ F ( P , z ) \right] = \lambda \mathsf { P } _ { \mathrm { t r a d e } } \frac { \sqrt { 2 \lambda } } { \sigma } \int _ { 0 } ^ { \infty } \frac { F _ { + } ( P , x + \gamma ) + F _ { - } ( P , - x - \gamma ) } { 2 } e ^ { - \sqrt { 2 \lambda } x / \sigma } d x .\tag{11}
$$

Proof. This result follows from standard properties of Poisson processes. The smoothing formula [e.g., Theorem 13.5.7, Brémaud, 2020] yields that, for $T > 0$ ，

$$
\mathsf { E } \left[ \mathsf { A R B } _ { T } \right] = \mathsf { E } \left[ \sum _ { i = 1 } ^ { N _ { T } } A ( P _ { \tau _ { i } } , z _ { \tau _ { i } ^ { - } } ) \right] = \mathsf { E } \left[ \int _ { 0 } ^ { T } A ( P _ { t } , z _ { t ^ { - } } ) d N _ { t } \right] = \mathsf { E } \left[ \int _ { 0 } ^ { T } A ( P _ { t } , z _ { t ^ { - } } ) \times \lambda d t \right] .
$$

Applying Tonelli’s theorem and the fundamental theorem of calculus,

$$
\operatorname* { l i m } _ { T \to 0 } \frac { \mathsf { E } \left[ \mathsf { A R B } _ { T } \right] } { T } = \operatorname* { l i m } _ { T \to 0 } \frac { \lambda } { T } \int _ { 0 } ^ { T } \mathsf { E } \left[ A ( P _ { t } , z _ { t ^ { - } } ) \right] d t = \lambda \mathsf { E } \left[ A ( P _ { 0 } , z _ { 0 ^ { - } } ) \right] ,
$$

and the result then follows from Theorem 1. The same argument applies to the intensity of the fee process. ■

## 4.3. Example: Constant Product Market Maker

Theorem 2 provides an exact, semi-analytic closed form expression for the rate of arbitrage profit, in terms of a certain Laplace transfrom of the functions $\{ A _ { \pm } ( P , \cdot ) \}$ . This expression can be evaluated as an explicit closed form for many CFMMs. For example, consider the case of constant product market makers:

Corollary 2. Consider a constant product market maker, with invariant $f ( x , y ) \triangleq { \sqrt { x y } } = L$ . Under the assumptions of Theorem ${ \mathcal { Q } } ,$ the intensity per dollar value in the pool of arbitrage profits and fees are given $b y ^ { 1 0 }$

$$
\begin{array} { r l } & { \frac { \overline { { \mathsf { A R B } } } } { \overline { { V ( P ) } } } = \left\{ \begin{array} { l l } { \frac { \sigma ^ { 2 } } { 8 } \times \mathsf { P _ { t r a d e } } \times \frac { e ^ { + \gamma / 2 } } { 1 - \sigma ^ { 2 } / ( 8 \lambda ) } } & { i f \sigma ^ { 2 } / 8 < \lambda , } \\ { + \infty } & { o t h e r w i s e , } \end{array} \right. } \\ & { \frac { \overline { { \mathsf { F E E } } } } { \overline { { V ( P ) } } } = \frac { \sigma ^ { 2 } } { 8 } \times \frac { e ^ { + \gamma / 2 } - e ^ { - \gamma / 2 } } { \gamma } \times \frac { 1 } { \left( 1 + \sigma / \left( \sqrt { 2 \lambda } \gamma \right) \right) \left( 1 + \sigma / \left( 2 \sqrt { 2 \lambda } \right) \right) } , } \end{array}
$$

where the quantities on the right side do not depend on the value of $P _ { 0 } = P$

The proof of Corollary 2 is deferred until Appendix D. Under the normalization of Corollary 2, where the intensity of arbitrage profits is normalized relative the pool value, the resulting quantity does not depend on the price. The same property will hold for the more general class of geometric

![](images/8a087da4cd6dfc78e5bac3ad7b552e9cae25dfb2044bd47df853e7839d353246.jpg)

![](images/910968729628d2efce4666764e205eb54e254388a8e738a8333ca1708158aaa9.jpg)  
(a) The normalized intensity of arbitrage profit ARB/V (P ) as a function of the fee γ.  
(b) The relative error of the approximation (12), i.e., $( \overline { { \mathsf { A R B } } } - \overline { { \mathsf { L V R } } } \times \mathsf { P _ { t r a d e } } ) / \overline { { \mathsf { A R B } } } ,$ as a function of the fee γ.  
Figure 5: The constant product market maker case, with $\sigma = 5 \%$ (daily) and mean interblock time $\Delta { \dot { t } } \triangleq \lambda ^ { - 1 } = 1 2$ (seconds).

mean market makers; this is analogous to the property that LVR is proportional to pool value for this class [Milionis et al., 2022].

As a comparison point, for a constant product market maker, Milionis et al. [2022] establish that

$$
{ \overline { { \mathsf { L V R } } } } \triangleq \operatorname* { l i m } _ { T  0 } { \frac { \mathsf { E } [ \mathsf { L V R } _ { T } ] } { T } } = { \frac { \sigma ^ { 2 } } { 8 } } \times V ( P ) ,
$$

so that, when $\sigma ^ { 2 } / 8 < \lambda$

$$
\overline { { \mathsf { A R B } } } = \overline { { \mathsf { U R } } } \times \mathsf { P } _ { \mathsf { t r a d e } } \times \underbrace { e ^ { + \gamma / 2 } } _ { \approx 1 + O ( \gamma ) } \times \underbrace { \frac { 1 } { 1 - \sigma ^ { 2 } / ( 8 \lambda ) } } _ { \approx 1 + O ( \lambda ^ { - 1 } ) } .
$$

Therefore, when fees are small $( \gamma  0 )$ and the block rate is high $( \lambda \to \infty )$ , we have the approximation

$$
\overline { { \mathsf { A R B } } } \approx \overline { { \mathsf { L V R } } } \times \mathsf { P } _ { \mathsf { t r a d e } } .\tag{12}
$$

In Figure 5b, we see that for typical parameter values this approximation is quite accurate, with a relative error of less that $1 0 ^ { - 2 }$

## 5. Asymptotic Analysis

In this section, we consider a fast block regime, where $\lambda \to \infty$ . In this setting, blocks are generated very quickly, or, equivalently, the interblock time $\Delta t \triangleq \lambda ^ { - 1 } \to 0$ is very small. First, we characterize asymptotic arbitrage profits in this regime:

Theorem 3. Define

$$
\bar { A } ( P , x ) \triangleq \frac { A _ { + } ( P , x + \gamma ) + A _ { - } ( P , - x - \gamma ) } { 2 } \geq 0 .
$$

Assume that, for each $P > 0 , \bar { A } ( P , \cdot )$ is twice continuously differentiable, and that there exists $A _ { 0 }$ and c (possibly depending on P ) such that

$$
\partial _ { x x } \bar { A } ( P , x ) \leq A _ { 0 } e ^ { c x } , \quad \forall x \geq 0 .\tag{13}
$$

Consider the fast block regime where $\lambda \to \infty$ . Then,

$$
\overline { { { \sf A R B } } } = { \frac { \sigma ^ { 2 } P } { 2 } } \times { \frac { y ^ { * / } \left( P e ^ { - \gamma } \right) + e ^ { + \gamma } \cdot y ^ { * / } \left( P e ^ { + \gamma } \right) } { 2 } } \times \mathsf { P } _ { \mathrm { t r a d e } } + o \left( \sqrt { \lambda ^ { - 1 } } \right) .\tag{14}
$$

Equation (14) highlights the dependence of arbitrage profits on the problem parameters. In the regime where volatility $\sigma$ is large, the fee $\gamma$ is small, and the block rate λ is high, we have that $\mathsf { P } _ { \mathrm { t r a d e } } \approx \eta ^ { - 1 } = \sigma \sqrt { \lambda ^ { - 1 } / 2 } / \gamma$ . This implies that arbitrage profits are proportional to the square root of the mean interblock time $( { \sqrt { \lambda ^ { - 1 } } } )$ , the cube of the volatility $( \sigma ^ { 3 } )$ , and the reciprocal of the fee $( \gamma ^ { - 1 } )$ . This result suggests that faster blockchains (higher λ) will result in reduced arbitrage profits. We discuss this result in more detail in Section 7.1.

The next result will similarly characterize fees in this regime:

Theorem 4. Define

$$
\bar { F } ( P , x ) \triangleq \frac { F _ { + } ( P , x + \gamma ) + F _ { - } ( P , - x - \gamma ) } { 2 } \geq 0 .
$$

Assume that, for each $P > 0 , \bar { F } ( P , \cdot )$ is continuously differentiable, and that there exists $F _ { 0 }$ and c (possibly depending on P ) such that

$$
\partial _ { x } \bar { F } ( P , x ) \leq F _ { 0 } e ^ { c x } , \quad \forall x \geq 0 .\tag{15}
$$

Consider the fast block regime where $\lambda \to \infty$ . Then, the instantaneous rate of fees (defined similarly to Theorem 2) is

$$
\overline { { { \sf F E } } } = \frac { \sigma ^ { 2 } P } { 2 } \times \frac { ( 1 - e ^ { - \gamma } ) y ^ { * \prime } ( P e ^ { - \gamma } ) + ( e ^ { + \gamma } - 1 ) y ^ { * \prime } ( P e ^ { + \gamma } ) } { 2 \gamma } \times \left( 1 - \sf P _ { t r a d e } \right) + o \left( 1 \right) .\tag{16}
$$

The proofs of Theorems 3 and 4 are deferred to Appendix E. Equation (13) is a mild technical condition bounding the convexity of the arbitrage profit as a function of the mispricing. Theorem 3 provides theoretical justification for the discussion in Section 1.2 comparing (1)–(2): we have that, for arbitrary AMMs satisfying the technical condition of (13), $\overline { { \mathsf { A R B } } } \approx \overline { { \mathsf { L V R } } } \times \mathsf { P _ { t r a d e } }$ when the fee γ is small in the fast block regime. Additionally, the instantaneous rate of fees is shown by Equation (16) to be $\overline { { \mathsf { F E E } } } \approx \overline { { \mathsf { L V R } } } \times \left( 1 - \mathsf { P } _ { \mathsf { t r a d e } } \right)$ when the fee γ is small in the fast block regime. The last two results mean that, conditioned on the fee γ being small in the fast block regime, ${ \overline { { \mathsf { A R B } } } } + { \overline { { \mathsf { F E E } } } } \approx { \overline { { \mathsf { L V R } } } } .$ , which can be interpreted as LVR being split among fees and arbitrage profits, according to $\mathsf { P _ { t r a d e } }$ . In particular, as the blocks become more and more frequent (for a fixed fee $\gamma )$ , LVR switches from arbitrage profits to fees, where it is eventually consumed.

Empirically, this decomposition that $\overline { { \mathsf { A R B } } } + \overline { { \mathsf { F E E } } } \approx \overline { { \mathsf { L V R } } }$ was implicitly validated in the original work of Milionis et al. [2022]. There, the authors empirically validated that delta-hedged LP P&L in a pool with fees matches the difference between total fees collected and LVR. This is consistent with our setting: since total fees can be decomposed into fees from noise traders plus fees from arbitrageurs, the difference between total fees collected and LVR is noise trading fees minus arbitrage profits.

## 6. Modeling of Fixed Gas Fees

In this section, we give a formulation of arbitrage profits that takes into account the presence of gas fees as costs for arbitrageurs, and analyze these profits in an asymptotic way as in Section 5. Gas fees are a cost required to be paid to include the arbitrage transaction in a block. From a financial perspective, they are fixed cost in that they do not depend on the size of a trade. Gas fees occur due to the competition of arbitrage transactions with other transactions to be included in a finite block with limited blockspace for transactions. While the proportional swap transaction fees determined by $( \gamma _ { + } , \gamma _ { - } )$ go to the AMM (a smart contract living at the application layer) where they are then distributed to LPs (cf. Section 2), gas fees go to the infrastructure, where they are typically earned by the producers of a block (also called the “validators” or “proposers” of a block). Intuitively, then, one might hope that gas fees acts as a second but analogous friction to arbitrage, with the main difference being the recipient of the fees paid by arbitrageurs (LPs vs. the protocol/validators). Our results formalize this intuition.

At each block, every arbitrage transaction must pay the gas fee which is constant for that block, just for interacting with the pool. Here, we are interpreting the fixed gas fee of a block as the market-clearing price for transaction inclusion in the block (i.e., for blockspace). Even though for a given block the gas fee is fixed, it can vary from block to block, due to dynamic pricing mechanisms of the underlying blockchain or competition with other transactions. Fixing the gas cost, and given a fixed price of the AMM and liquidity, charging a fixed gas fee (i.e., reducing any arbitrageur’s profit by this fixed amount) is equivalent to an additional threshold $\delta _ { + } \geq 0$ (in the same units as log-mispricing, e.g., basis points) on top of the boundary $\gamma _ { + }$ (and $\delta _ { - } \geq 0$ on top of the boundary $\gamma _ { - }$ −, respectively) that needs to be overcome in the mispricing process for a profitable trade to exist for the arbitrageur. More details on this are provided in Appendix I. For analytical tractability, we will make the assumption that $\delta _ { + }$ and $\delta _ { - }$ − are constant, independent of the current price of the AMM or liquidity thereof. In Appendix I, we also discuss why in practice this is a good approximation.

Formally, as per our model of Lemma 2, suppose that a block is generated at time t, so that the arbitrageur observes the price $P _ { t - }$ . According to the analysis there, we had that if $z _ { t ^ { - } } > + \gamma _ { + }$ ， then the pool is underpriced, and the arbitrageur can buy from the pool and earn a profit. Once they committed to buying from the pool, they were doing so until their marginal profit was zero, i.e., until $z _ { t } ~ = ~ + \gamma _ { + }$ . To incorporate gas fees, we define the quantities of the new boundaries $\bar { \gamma } _ { + } \triangleq \gamma _ { + } + \delta _ { + } \geq \gamma _ { + }$ and $\bar { \gamma } _ { - } \triangleq \gamma _ { - } + \delta _ { - } \geq \gamma .$ −, such that the effect to the mispricing process will now be:

Assumption 3. The mispricing process with gas fees follows the rules:

$I f z _ { t ^ { - } } > \bar { \gamma } _ { + } = \gamma _ { + } + \delta _ { + }$ , we will have that $z _ { t } = \gamma _ { + }$ (after the arbitrageur’s trade).

$I f z _ { t ^ { - } } < - \bar { \gamma } _ { - } = - \gamma _ { - } - \delta _ { - }$ , we will have that $z _ { t } = - \gamma _ { - }$ (after the arbitrageur’s trade).

• Otherwise, $z _ { t ^ { - } } = z _ { t }$

Summary of results. Our theorems below in the fast block regime indicate the following observations: first, the losses to liquidity providers (measured as the sum of the profits of the arbitrageurs and the gas fees)11 increase with increasing gas fees. This means that the lower the gas fees, the smaller the losses to LPs. Effectively, gas fees act as a friction to arbitrageurs which delays profits and artificially decreases competition in a similar manner to the action of block times, analyzed in Section 5. Second, arbitrageurs trade less frequently with higher gas fees, but make higher profits per arbitrage trade. Third, we can quantify the instantaneous rates of each of: the arbitrage profits (ARB), the trading fees paid to the AMM (FEE), and the gas fees paid to validators (GAS) as a split of LVR akin to the split we had without gas fees. This shows again that LVR is the fundamental quantity due to the stale information induced by the structure of any AMM. Finally, asymptotically in the fast block regime, all of the LP losses leak to the validators in gas. In effect, in this regime, arbitrageurs compete all of their profits away to validators.

## 6.1. Stationary distribution of mispricing

We continue the rest of the section (for ease of exposition) under the symmetric Assumptions 2 and 3 along with $\delta _ { + } = \delta _ { - } \triangleq \delta$ , so that $\bar { \gamma } _ { + } = \bar { \gamma } _ { - } \triangleq \bar { \gamma }$ . The following result characterizes the stationary distribution of the mispricing process in this case. This result is analogous to Theorem 1, however it is not mathematically equivalent to setting a fee of ${ \bar { \gamma } } \triangleq \gamma + \delta$ in that setting. In particular, in Theorem 1, the threshold that determines whether a trade occurs (γ) is the same as the mispricing the arbitrageur trades to (cf. Lemma 2) if the threshold is exceeded. Under Assumption 3, however the threshold that determines trading $( { \bar { \gamma } } { \overset { \Delta } { = } } \gamma + \delta )$ is different than the trade-to midpricing (γ). We defer the proof to Appendix F.

Theorem 5 (Stationary Distribution of Mispricing). Under Assumptions 2 and 3, the process $z _ { t }$ is an ergodic process on R, with unique invariant distribution $\pi ( \cdot )$ given by the density

$$
p _ { \pi } ( z ) = \left\{ \begin{array} { l l } { \pi _ { + } \times p _ { \eta / \bar { \gamma } } ^ { \mathrm { e x p } } ( z - \bar { \gamma } ) } & { i f ~ z > + \bar { \gamma } , } \\ { \pi _ { + } \times \frac { \eta } { \bar { \gamma } } \Big [ u ( z + \bar { \gamma } ) - u ( z - \bar { \gamma } ) } \\ { \qquad + \frac { \eta } { \bar { \gamma } } \left( r ( z + \bar { \gamma } ) + r ( z - \bar { \gamma } ) - r ( z + \gamma ) - r ( z - \gamma ) \right) \Big ] } & { i f ~ z \in [ - \bar { \gamma } , + \bar { \gamma } ] , } \\ { \pi _ { - } \times p _ { \eta / \bar { \gamma } } ^ { \mathrm { e x p } } ( - \bar { \gamma } - z ) } & { i f ~ z < - \bar { \gamma } , } \end{array} \right.
$$

for $z \in \mathbb { R }$ , where $u ( x ) \triangleq \mathbb { I } _ { \{ x \geq 0 \} }$ is the standard unit step function, and $r ( x ) \triangleq$ max(x, 0) is the standard ramp function. Here, we re-define the composite parameter $\eta = \sqrt { 2 \lambda } \bar { \gamma } / \sigma$ . The probabilities $\pi _ { - } , \pi _ { + }$ are given by

$$
\pi _ { + } \triangleq \pi \big ( ( + \bar { \gamma } , + \infty ) \big ) = \pi _ { - } \triangleq \pi \big ( ( - \infty , - \bar { \gamma } ) \big ) = \frac { 1 } { 2 + 2 \eta + \eta ^ { 2 } \left( 1 - \left( \frac { \gamma } { \gamma + \delta } \right) ^ { 2 } \right) } .\tag{17}
$$

Final ly, $p _ { \eta / \bar { \gamma } } ^ { \mathrm { e x p } } ( x ) \ \triangleq \ ( \eta / \bar { \gamma } ) e ^ { - ( \eta / \bar { \gamma } ) x }$ is the density of an exponential distribution over $x \ge 0$ with parameter $\eta / \bar { \gamma } = \sqrt { 2 \lambda } / \sigma$

![](images/7e04d361b33e514e35129823fdfe4e2b60d7840b49ebde8fc4224df4bae2c99f.jpg)  
Figure 6: The density $p _ { \pi } ( z )$ of the stationary distribution π(·) of mispricing z with gas fees, illustrating trade and no-trade regions for an arbitrageur. Comparing to Figure 3, notice the regions $[ - \gamma - \delta , - \gamma ]$ and $[ + \gamma , + \gamma + \delta ]$ where no arbitrage trade happens and which have a different, trapezoid shape.

The stationary distribution is illustrated in Figure 6. Comparings the stationary distributions of Theorem 1 and Theorem 5, observe that they both have exponential tails for large mispricings z, as well as a uniform density near $z = 0$ . However, the distribution in Theorem 5 introduces two additional intervals $[ - \gamma - \delta , - \gamma ]$ and $[ + \gamma , + \gamma + \delta ]$ where the density is linear.

Probability of trade. Note that according to Equation (17), the probability of a profitable trade, i.e., $\pi _ { + } + \pi _ { - }$ , is decreasing in gas fees.

## 6.2. Asymptotic Analysis

In this section, we will characterize the asympotic rate of arbitrage profits, trading fees, gas fees, and LP losses, in the fast block regime, similar to the analysis of Section 5. Proofs are relegated to Appendix G.

## 6.2.1. Arbitrage Profits and Trading Fees

We characterize arbitrage profits as follows:

Theorem 6. Re-define

$$
\bar { A } ( P , x ) \triangleq \frac { A _ { + } ( P , x + \bar { \gamma } ) + A _ { - } ( P , - x - \bar { \gamma } ) } { 2 } \geq 0 ,
$$

where the rate of arbitrageurs’ profits is re-defined to exclude gas fees paid to validators, $i . e .$

$$
A _ { + } ( P , z ) \triangleq \left[ P \left\{ x ^ { * } \left( P e ^ { - z } \right) - x ^ { * } \left( P e ^ { - \gamma } \right) \right\} + e ^ { + \gamma } \left\{ y ^ { * } \left( P e ^ { - z } \right) - y ^ { * } \left( P e ^ { - \gamma } \right) \right\} - g _ { + } \right] \mathbb { I } _ { \{ z > + \gamma \} } \geq 0 ,
$$

$$
A _ { - } ( P , z ) \triangleq \left[ e ^ { + \gamma } P \left\{ x ^ { * } \left( P e ^ { - z } \right) - x ^ { * } \left( P e ^ { + \gamma } \right) \right\} + \left\{ y ^ { * } \left( P e ^ { - z } \right) - y ^ { * } \left( P e ^ { + \gamma } \right) \right\} - g _ { - } \right] \mathbb { I } _ { \{ z < - \widetilde { \gamma } \} } \geq 0 ,
$$

where $g _ { + } , g$ − are the expressions preceding each one, evaluated at $z = + \bar { \gamma } , z = - \bar { \gamma }$ respectively, i.e.,

$$
g _ { + } \triangleq P \left\{ x ^ { * } \left( P e ^ { - \gamma - \delta } \right) - x ^ { * } \left( P e ^ { - \gamma } \right) \right\} + e ^ { + \gamma } \left\{ y ^ { * } \left( P e ^ { - \gamma - \delta } \right) - y ^ { * } \left( P e ^ { - \gamma } \right) \right\} \geq 0 , \ a n d
$$

$$
g _ { - } \triangleq e ^ { + \gamma } P \left\{ x ^ { * } \left( P e ^ { + \gamma + \delta } \right) - x ^ { * } \left( P e ^ { + \gamma } \right) \right\} + \left\{ y ^ { * } \left( P e ^ { + \gamma + \delta } \right) - y ^ { * } \left( P e ^ { + \gamma } \right) \right\} \geq 0 .
$$

Here, $( g _ { + } , g _ { - } )$ are the gas costs $( + \delta , - \delta )$ valued in the numéraire rather than as a proportional fee.

Assume that, for each $P > 0 , \bar { A } ( P , \cdot )$ is continuously differentiable, and that there exists $A _ { 0 }$ and c (possibly depending on P ) such that

$$
\partial _ { x } \bar { A } ( P , x ) \leq A _ { 0 } e ^ { c x } , \quad \forall x \geq 0 .\tag{18}
$$

Consider the fast block regime where $\lambda \to \infty$ . Then,

$$
\overline { { \mathsf { A R B } } } = \frac { \sigma ^ { 2 } P } { 2 } \times \frac { y ^ { \ast \prime } \left( P e ^ { - \gamma - \delta } \right) + e ^ { + \gamma + \delta } \cdot y ^ { \ast \prime } \left( P e ^ { + \gamma + \delta } \right) } { 2 } \times \left( 1 - e ^ { - \delta } \right) \times \frac { \sqrt { 2 \lambda } } { \sigma } \times \mathsf { P _ { t r a d e } } + o \left( \sqrt { \lambda ^ { - 1 } } \right)\tag{19}
$$

Next, we consider the trading fees generated:

Theorem 7. Re-define

$$
\bar { F } ( P , x ) \triangleq \frac { F _ { + } ( P , x + \bar { \gamma } ) + F _ { - } ( P , - x - \bar { \gamma } ) } { 2 } \geq 0 ,
$$

where

$$
\begin{array} { l } { { F _ { + } ( P , z ) \triangleq - \left( e ^ { + \gamma } - 1 \right) \left[ y ^ { * } \left( P e ^ { - z } \right) - y ^ { * } \left( P e ^ { - \gamma } \right) \right] \mathbb { I } _ { \{ z > + \bar { \gamma } \} } \geq 0 , \ a n d } } \\ { { F _ { - } ( P , z ) \triangleq - \left( e ^ { + \gamma } - 1 \right) P \left[ x ^ { * } \left( P e ^ { - z } \right) - x ^ { * } \left( P e ^ { + \gamma } \right) \right] \mathbb { I } _ { \{ z < - \bar { \gamma } \} } \geq 0 . } } \end{array}
$$

Assume that, for each $P > 0 , \bar { F } ( P , \cdot )$ is continuous, and that there exists $F _ { 0 }$ and $c \ ( p o s s i b l y$ depending on P ) such that

$$
\bar { F } ( P , x ) \leq F _ { 0 } e ^ { c x } , \quad \forall x \geq 0 .\tag{20}
$$

Consider the fast block regime where $\lambda \to \infty$ . Then,

$$
\mathsf { F E E } = ( 1 - e ^ { \gamma } ) \times \frac { P \cdot \left( x ^ { \ast } \left( P e ^ { \gamma + \delta } \right) - x ^ { \ast } \left( P e ^ { \gamma } \right) \right) + y ^ { \ast } \left( P e ^ { - \gamma - \delta } \right) - y ^ { \ast } \left( P e ^ { - \gamma } \right) } { 2 } \times \lambda \mathsf { P } _ { \mathrm { t r a d e } } + o \left( 1 \right)\tag{21}
$$

No-gas fee limits of ARB, FEE. When there is no gas fee, Equations (19) and (21) of Theorems 6 and 7 yield the same results as our previous Equations (14) and (16) of Theorems 3 and 4 respectively. To show that, one needs to be careful in handling the asymptotic limits as $\lambda \to \infty$ . For the full details, please reference Appendix H, where this consistency is proven.

Asymptotic block time rate with gas fees. Comparing with the previous setting where there is no gas fee (of Theorem 3), the rate of the decrease of arbitrage profits with block times remains $\Theta ( \sqrt { \lambda ^ { - 1 } } )$ . It is interesting to observe that the rate of the probability of a profitable arbitrage decreases, and the rate of the arbitrage profits conditioned on trade increases. More specifically, the rate of the probability of a profitable arbitrage becomes $\Theta ( \lambda ^ { - 1 } )$ , and the rate of arbitrage profits conditioned on a profitable trade becomes $\Theta ( { \sqrt { \lambda } } )$ . Due to the differences illustrated in Figure 6, since gas fees are a roablock to arbitrageur profitability, they make arbitrage trades more infrequent, and at the same time, conditioned on trade, more profitable, because (in the same intuitive fashion as block times) it delays trading on the AMM. Due to prices deviating farther (i.e., varying with the delay time), there’s higher expected rate of return to be made by the arbitrageurs.

## 6.2.2. Gas Fees and LP Losses

We now compute the gas fees that go to the validators, as follows. This is a straightforward corollary of Theorem 5.

Corollary 3 (Gas fees). Under Assumptions 2 and 3, in the setting described by Section 4.2, the

instantaneous rate of the gas fees that go to validators is given $b y ^ { 1 2 }$

$$
\begin{array} { r l } { { \sf G A S } \triangleq } & { \lambda \cdot ( g _ { + } \cdot \pi _ { + } + g _ { - } \cdot \pi _ { - } ) } \\ { = } & { \frac { \lambda } { 1 + ( \gamma + \delta ) \sqrt { 2 \lambda } / \sigma + \lambda ( ( \gamma + \delta ) ^ { 2 } - \gamma ^ { 2 } ) / \sigma ^ { 2 } } \cdot ( \frac { P \{ x ^ { * } ( P e ^ { - \gamma - \delta } ) - x ^ { * } ( P e ^ { - \gamma } ) \} } { 2 }  } \\ & {  + \frac { e ^ { + \gamma } \{ y ^ { * } ( P e ^ { - \gamma - \delta } ) - y ^ { * } ( P e ^ { - \gamma } ) \} } { 2 } + \frac { P e ^ { + \gamma } \{ x ^ { * } ( P e ^ { + \gamma + \delta } ) - x ^ { * } ( P e ^ { + \gamma } ) \} } { 2 }  } \\ & { + \frac { y ^ { * } ( P e ^ { + \gamma + \delta } ) - y ^ { * } ( P e ^ { + \gamma } ) } { 2 } ) , } \end{array}\tag{22}
$$

where $g _ { + } , g _ { - }$ are the instantaneous transactional costs at the mispricing boundaries that go to the validators (formally defined in Theorem 6).

In the limit of the fast block regime where $\lambda \to \infty$ , the first factor of the product (which is the only λ-dependent factor) of Equation (22) becomes

$$
{ \frac { \sigma ^ { 2 } } { ( \gamma + \delta ) ^ { 2 } - \gamma ^ { 2 } } } ,
$$

and separately, for small gas fees, the second factor of Equation (22) becomes

$$
\frac { P \delta ^ { 2 } } { 2 } \cdot \frac { y ^ { * \prime } \left( P e ^ { - \gamma } \right) + e ^ { + \gamma } \cdot y ^ { * \prime } \left( P e ^ { + \gamma } \right) } { 2 } .
$$

Losses in the asymptotically fast regime. Comparing Equations (14) and (22) in the asymptotic fast block regime, if we take the limit $\lambda \to \infty$ , the arbitrageurs do not make any profits, but there is leakage to validators in the form of positive gas fees paid. Therefore, in this limit of fast blocks, LPs still lose a constant amount of money, but this is taken by validators rather than arbitrageurs. More specifically, the rate with respect to λ is Θ(1). We highlight that this observation does not require any assumption of small gas fees.

Corollary 4 (Gas fees δ-asymptotics). Consider the limit of the fast block regime where $\lambda \to \infty$ , and small gas fees. Then, Equation (22) becomes

$$
\overline { { { \mathsf { G A S } } } } = \frac { \sigma ^ { 2 } P } { 2 } \times \frac { y ^ { * \prime } \left( P e ^ { - \gamma } \right) + e ^ { + \gamma } \cdot y ^ { * \prime } \left( P e ^ { + \gamma } \right) } { 2 } \times \frac { \delta } { 2 \gamma } + o ( \delta ) .\tag{23}
$$

Parametric dependence of asymptotics. From Equation (23), we see that the gas fees given to validators in the fast block regime with small fee are proportional to the ratio of the gas margin to the roundtrip swapping fee $\delta / ( 2 \gamma )$ , the quadratic variation of the price process, as well as the marginal liquidity available on the AMM at the mispricing boundary. Note that this quantity is bounded away from zero even as the block rate tends to infinity, thus LPs lose money to validators no matter how fast the block arrival rate is. This loss to validators decreases with lower gas fees, asymptotically vanishing with zero gas fees.

LP losses. Finally, in a similar manner to Theorem 2, we define the rate of the profits of LPs as LP. Here, the losses of LPs are no longer just the trading profits of arbitrageurs (because validators also exist here which are obtaining their own gas fees), and are thus going to be lost to both arbitrage profits and total gas fees, i.e., Equations (19) and (22), so that $\overline { { \mathsf { L P } } } = - \overline { { \mathsf { A R B } } } - \overline { { \mathsf { G A S } } }$ . Therefore, in the fast block regime, the dominating term will be the $\Theta ( 1 )$ term of the gas fees given to validators, according to Equation (23). In particular, following the observation of the previous paragraph, lower gas fees yield less losses for LPs.

LVR as the total sum due to stale prices. In the fast block regime with small fees (δ as well as γ), from Equations (19), (21) and (23) as well as Corollary 3, we have that ${ \overline { { \mathsf { A R B } } } } + { \overline { { \mathsf { F E E } } } } + { \overline { { \mathsf { G A S } } } } = { \overline { { \mathsf { L V R } } } }$ namely that the entire quantity existing in the system due to the informational lag imposed by the stale prices of AMMs is LVR. In particular, when fees (gas and trading) are small but finite, in the fast block regime, the split is as follows:

$$
\overline { { \mathsf { A R B } } } \approx \overline { { \mathsf { L V R } } } \times \frac { \sqrt { \sigma ^ { 2 } / ( 2 \lambda ) } } { \gamma + \frac { \delta } { 2 } }\tag{24}
$$

$$
\overline { { \mathsf { G A S } } } \approx \overline { { \mathsf { L V R } } } \times \frac { \delta } { 2 \gamma } , \mathrm { ~ a n d }\tag{25}
$$

$$
\overline { { \mathsf { F E E } } } \approx \overline { { \mathsf { L V R } } } \times \left( 1 - \frac { \delta } { 2 \gamma } - \frac { \sqrt { \sigma ^ { 2 } / ( 2 \lambda ) } } { \gamma + \frac { \delta } { 2 } } \right) .\tag{26}
$$

Example calculations of the split into ARB and $\overline { { \mathsf { G A S } } }$ are shown in Table 2 for $\sigma = 5 \% ( \mathrm { d a i l y } )$ volatility and varying mean interblock times $\Delta t \triangleq \lambda ^ { - 1 }$ , fee levels γ, and gas fees δ. For these calculations, we use the formulas from Equations (19) and (22) that make fewer asymptotic assumptions than the ones above which show the parametric dependencies. Also, as per the discussion in $\mathrm { A p \mathrm { - } }$ pendix I, the gas mispricing δ mostly depends on $g / { \sqrt { L } } ,$ and is not expected to vary much with either the proportional trading fee of the AMM or the block time.13

## 7. Implications

This section discusses the practical implications of our model for AMM design and blockchain architecture.

<table><tr><td>△t\</td><td>1bp</td><td>5bp</td><td>10bp</td><td>30 bp</td><td>100 bp</td></tr><tr><td>12 sec</td><td>(34.5%, 37.3%)</td><td>(23.3%, 25.1%)</td><td>(16.5%, 17.8%)</td><td>(7.6%, 8.3%)</td><td>(2.7%, 2.9%)</td></tr><tr><td>2 sec</td><td>(22.0%, 58.4%)</td><td>(13.6%, 36.1%)</td><td>(9.2%, 24.4%)</td><td>(4.0%,10.7%)</td><td>(1.4%, 3.6%)</td></tr><tr><td> 50 msec</td><td>(4.6%, 77.5%)</td><td>(2.7%, 45.3%)</td><td>(1.8%, 29.8%)</td><td>(0.8%, 12.6%)</td><td>(0.2%, 4.2%)</td></tr></table>

(a) Table varying mean inter-block times $\Delta t \triangleq \lambda ^ { - 1 }$ and fee levels γ (in basis points), given gas fee $\delta = 9 \mathrm { b p . } ^ { 1 4 }$

<table><tr><td>△t\</td><td>1 bp</td><td>5 bp</td><td>10bp</td><td>30bp</td><td>100 bp</td></tr><tr><td>12 sec</td><td>(2.4%, 0.3%)</td><td>(6.4%,3.8%)</td><td>(7.8%, 9.4%)</td><td>(7.7%, 27.8%)</td><td>(4.8%, 58.4%)</td></tr><tr><td>2 sec</td><td>(2.0%, 0.6%)</td><td>(3.8%, 5.6%)</td><td>(4.0%, 11.9%)</td><td>(3.5%, 30.9%)</td><td>(2.1%, 60.8%)</td></tr><tr><td>50 msec</td><td>(0.7%, 1.3%)</td><td>(0.8%, 7.3%)</td><td>(0.7%,13.9%)</td><td>(0.6%,32.9%)</td><td>(0.3%,62.2%)</td></tr></table>

(b) Table varying mean inter-block times $\Delta t \triangleq \lambda ^ { - 1 }$ and gas fee levels δ (in basis points), given trading fee $\gamma = 3 0 \mathrm { { b p } }$  
Table 2: The percentage split of $\overline { { \mathsf { L V R } } } = \overline { { \mathsf { A R B } } } + \overline { { \mathsf { F E E } } } + \overline { { \mathsf { G A S } } }$ into (ARB, GAS) for each entry of the table respectively, given asset price volatility σ = 5% (daily). The two tables vary different parameters.

## 7.1. Blockchain Architecture Implications

Our results provide clear guidance for reducing arbitrage profits and improving LP performance. The key insight is that faster block times directly reduce arbitrage profits through the $\mathsf { P _ { t r a d e } }$ factor, which decreases as λ increases (or, equivalently, as the mean block-time $\Delta t \triangleq \lambda ^ { - 1 }$ decreases). In particular, arbitrage profits per unit time scale according to $\Delta t ^ { 1 / 2 }$ , while arbitrage profits per block scale according to $\Delta t ^ { 3 / 2 }$ . This suggests that blockchain designers should prioritize faster block times to protect LPs from adverse selection. Similarly, blockchain designers can reduce arbitrage profits by reducing gas fees.

Indeed, in our model, arbitrage profits go to zero in the limit as $\Delta t \to 0$ . However, this is likely an artifact of the fact that prices are diffusions in our model, and are thus continuous. In reality, at very short time scales, market prices are better described with discountinous jump-diffusion processes. Such processes may exhibit greater movements over short time horizons and result in larger arbitrage profits.

Our results have been empirically validated by Fritsch and Canidio [2024]. They simulated arbitrage profits on an AMM against real world asset prices. Their work tests two assumptions made in this paper: the assumption of Poisson block times (they assume deterministic block times), and the assumption that asset prices follow a diffusion process (they used historical data from the Binance exchange). In their simulation, they considered counterfactuals involving varying the block time. With respect to the “square-root” decay of arbitrage profits predicted by the present paper, they conclude

While our empirical findings come close to [the square model of Milionis et al.] for most pairs and block times larger than 1s, we observe a different regime for block times shorter than 1s. More precisely, arbitrage profits appear to decline more slowly than the theoretical model would suggest.

These empirical results suggest our model is useful in a broad range of settings. However, on sub-second time scales, it is likely that the absence of jumps in our model limits its reach. An interesting direction for future exploration would be to model arbitrage profits when the asset price follows a jump process.

Our model of arbitrage profits also motivates the discussion of “multi-block” MEV. Here, consider a situation where a single agent controls many blocks in a row. By censoring the arbitrage trades of other agents, this agent can effectively increase the block time. Since the arbitrage profits increase as the block time increases, the agent now has incentive to seek control on contiguous blocks in order to censor the competing trades of others. Consensus mechanisms should factor these incentives in their design.

## 7.2. Pricing Accuracy

Rao and Shah [2023] suggest a trade-off for AMM designers between pricing accuracy, measured by the standard deviation of mispricing $\sigma _ { z }$ , and arbitrage profits. Setting fees that are low ensures accurate prices, but results in high arbitrage profits, while setting fees that are high has the opposite effect.

In our setting, we can crisply and analytically quantify this trade-off. Namely, the standard deviation of mispricing can be computed by Corollary 1, while the arbitrage profits can be computed by Theorem 2 (exactly) or Theorem 3 (asymptotically).

Figure 7 illustrates this trade-off for a constant product market maker, where the arbitrage profits are computed exactly using Corollary 2. This figure illustrates two bounds in the low fee regime $( \gamma \to 0 )$ . First, as $\gamma \to 0 , \overline { { \mathsf { A R B } } } / V ( P ) \uparrow \overline { { \mathsf { L V R } } } / V ( P ) = \sigma ^ { 2 } / 8 .$ . In this sense, LVR captures the worse case loss to arbitrageurs. Second, as $\gamma  0 , \sigma _ { z } \downarrow \sigma \sqrt { \lambda ^ { - 1 } }$ . The latter quantity is the standard deviation of log-price changes over the mean interblock time $\Delta t \ \triangleq \lambda ^ { - 1 }$ This is the minimal mispricing error forced by the discrete nature of the blockchain.

## 7.3. LP P&L Decomposition

In this section, we consider the original liquidity provider profit and loss decomposition framework established by Milionis et al. [2022]. This foundational work provides the theoretical basis for understanding how LPs generate returns in automated market makers. We extend this framework to incorporate our structurally micro-founded model for arbitrage profits, and discuss how this can be utilized in broader settings to understanding LP economics.

Noise Traders. First, we will augment our model to incorporate a population of AMM-specific noise traders. Noise traders trade only in the AMM, and trade for totally idiosyncratic reasons (e.g., convenience of executing on chain) and not for informational reasons.

While noise traders’ trades have an initial impact on AMM pool prices, these effects are mitigated by arbitrageurs, who immediately move (or, “backrun”) the AMM so that its marginal price (net of fees) is consistent with the external price. For tractability, we will make the simplifying assumption that noise traders do not have an impact on the price dynamics of the asset. Thus, from the $\mathrm { L P } { \mathrm { { s } } }$ perspective, noise traders simply contribute a flow of fees. We denote by ${ \mathsf { N T } } _ { - } { \mathsf { F E } } { \mathsf { E } } _ { N }$ the total fees collected from noise traders over the first N blocks.

![](images/5631f5b25fad055a117f6e674a011ce45916863bfb48a4a891361569422e308e.jpg)  
Figure 7: Efficient frontier between mispricing error and arbitrage profits for different choices of fees, for a constant product market maker. Here, we set $\sigma = 5 \%$ (daily) and $\lambda ^ { - 1 } = 1 2$ (seconds). The horizontal axis is the standard deviation of the steady state pool mispricing, $\sigma _ { z }$ . The vertical axis is the intensity per unit time of arbitrage profits per dollar value of the pool, ${ \overline { { \mathsf { A R B } } } } / V ( P )$

Rebalancing Strategy. The rebalancing strategy is a self-financing strategy that takes exactly the same position in the risky asset as the AMM pool, but does so at external market prices. We denote by $R _ { N }$ the rebalancing strategy P&L over the first N blocks. This is given by

$$
R _ { N } = \sum _ { i = 0 } ^ { N - 1 } x ^ { * } \left( \tilde { P } _ { \tau _ { i } } \right) \left( P _ { \tau _ { i + 1 } } - P _ { \tau _ { i } } \right) ,
$$

where $\tilde { P } _ { t }$ is the spot price of the AMM pool at time t, and $\{ \tau _ { i } \}$ is the sequence of block times.

LP P&L Decomposition. Following Milionis et al. [2022], we decompose the LP P&L over the first N blocks into three components: the rebalancing strategy P&L (i.e., directional P&L from the holdings of the AMM pool), the noise trader fees (i.e., pure revenue from noise traders), and the arbitrage profits (i.e., the value extracted by arbitrageurs from the LP position), according to

$$
\mathrm { L P } \mathsf { P } \& \mathsf { L } _ { N } = { \boldsymbol { R } } _ { N } + \mathsf { N T } \_ { \mathsf { F E E } } - \mathsf { A R B } _ { N } .
$$

The rebalancing strategy can be perfectly hedged through delta-hedging techniques. By taking offsetting positions in the underlying asset, LPs can eliminate the directional risk associated with price movements. This leaves only the fee revenue from noise traders minus the arbitrage profits as the net economic benefit to the LP. In expectation, this is given by

$$
\mathsf { E } \left[ \mathrm { d e l t a - h e d g e d ~ L P ~ P \& L } _ { N } \right] = \mathsf { E } \left[ \mathsf { N T \_ F E } _ { N } \right] - \mathsf { E } \left[ \mathsf { A R B } _ { N } \right] .\tag{27}
$$

Applications. Our paper provides a structural model for quantifying expected arbitrage profits $\left( \mathsf { E } [ \mathsf { A R B } _ { N } ] \right)$ , which represents the second term in (27). This structural approach can be combined with reduced-form models of noise trader activity to create a comprehensive framework for understanding LP economics. The structural model allows for micro-founded predictions of arbitrage costs under different market conditions and parameter settings. For example:

• Consider a setting where a monopolist LP wants to set optimal fees. Such an agent would pick the fees to maximize (27). This LP faces a trade-off: higher fees reduce noise trader activity (decreasing $\mathsf { E } \left[ \mathsf { N T \_ F E E } _ { N } \right] )$ but also reduce arbitrage profits (decreasing $\mathsf { E } \left[ \mathsf { A R B } _ { N } \right] )$ . Our model provides the analytical framework for understanding how fees affect the second term of (27), allowing the LP to optimize the fee level that maximizes net revenue.

• From a modeling perspective, our framework enables analysis of counterfactual equilibria in AMM markets. For instance, we can determine the equilibrium level of liquidity that would emerge if fees were changed. In a competitive market with free entry and exit of LPs, economic theory suggests that the delta-hedged LP P&L should be zero in equilibrium. Our model provides the analytical foundation for the arbitrage cost component of this equilibrium condition, allowing researchers to predict how changes in market parameters affect equilibrium liquidity levels. This approach is taken by Adams et al. [2024], for example.

## Acknowledgments

The authors wish to thank Nihar Shah, Rithvik Rao, Alexander Nezlobin, and Dan Robinson for helpful comments. The first author is supported in part by NSF awards CNS-2212745, CCF-2332922, CCF-2212233, DMS-2134059, and CCF-1763970, by an Onassis Foundation Scholarship, and an A.G. Leventis educational grant. The third author’s research at Columbia University is supported in part by NSF awards CCF-2006737 and CNS-2212745.

## Disclosures

The second author is an advisor to fintech companies. The third author is Head of Research at a16z crypto, which is an investor in various decentralized finance projects, including Uniswap, as well as in the crypto ecosystem more broadly (for general a16z disclosures, see https://www.a16z.com/disclosures/). Notwithstanding, the ideas and opinions expressed herein are those of the authors, rather than of any companies or their affiliates.

## References

Austin Adams, Ciamac C Moallemi, Sara Reynolds, and Dan Robinson. am-amm: An auction-managed automated market maker. arXiv preprint arXiv:2403.03367, 2024.

Hayden Adams, Noah Zinsmeister, and Dan Robinson. Uniswap v2 core, 2020.

Hayden Adams, Noah Zinsmeister, Moody Salem, River Keefer, and Dan Robinson. Uniswap v3 core, 2021.

Guillermo Angeris and Tarun Chitra. Improved price oracles: Constant function market makers. In Proceedings of the 2nd ACM Conference on Advances in Financial Technologies, pages 80–91, 2020.

Guillermo Angeris, Alex Evans, and Tarun Chitra. Replicating market makers. arXiv preprint arXiv:2103.14769, 2021a.

Guillermo Angeris, Alex Evans, and Tarun Chitra. Replicating monotonic payoffs without oracles. arXiv preprint arXiv:2111.13740, 2021b.

Pierre Brémaud. Markov chains: Gibbs fields, Monte Carlo simulation, and queues, volume 31. Springer Science & Business Media, 2nd edition, 2020.

Joseph Clark. The replicating portfolio of a constant product market. Available at SSRN 3550601, 2020.

Philip Daian, Ittay Goldfeder, Tyler Kell, Yunqi Li, Xueyuan Zhao, Iddo Bentov, Lorenz Breidenbach, and Ari Juels. Flash boys 2.0: Frontrunning, transaction reordering, and consensus instability in decentralized exchanges. In Proceedings of the 2020 ACM SIGSAC Conference on Computer and Communications Security, pages 910–928. ACM, 2020.

Richard Dewey and Craig Newbold. The pricing and hedging of constant function market makers. Working paper, 2023.

Alex Evans. Liquidity provider returns in geometric mean markets. arXiv preprint arXiv:2006.08806, 2020.

Alex Evans, Guillermo Angeris, and Tarun Chitra. Optimal fees for geometric mean market makers. In International Conference on Financial Cryptography and Data Security, pages 65–79. Springer, 2021.

Robin Fritsch and Andrea Canidio. Measuring arbitrage losses and profitability of amm liquidity. In Companion Proceedings of the ACM Web Conference 2024, pages 1761–1767, 2024.

J Michael Harrison. Brownian models of performance and control. Cambridge University Press, 2013.

S. P. Meyn and R. L. Tweedie. Stability of Markovian processes III: Foster–Lyapunov criteria for continuoustime processes. Advances in Applied Probability, 25(3):518–548, 1993.

Jason Milionis, Ciamac C. Moallemi, Tim Roughgarden, and Anthony Lee Zhang. Quantifying loss in automated market makers. In Proceedings of the 2022 ACM CCS Workshop on Decentralized Finance and Security, DeFi’22, page 71–74, New York, NY, USA, 2022. Association for Computing Machinery. ISBN 9781450398824. doi: 10.1145/3560832.3563441. URL https://doi.org/10.1145/3560832.3563441.

Jason Milionis, Ciamac C. Moallemi, and Tim Roughgarden. Complexity-Approximation Trade-Offs in Exchange Mechanisms: AMMs vs. LOBs. In Financial Cryptography and Data Security, pages 326–343, Cham, 2023. Springer Nature Switzerland. ISBN 978-3-031-47754-6.

Jason Milionis, Ciamac C. Moallemi, and Tim Roughgarden. A Myersonian Framework for Optimal Liquidity Provision in Automated Market Makers. In 15th Innovations in Theoretical Computer Science Conference (ITCS 2024), Leibniz International Proceedings in Informatics (LIPIcs), Dagstuhl, Germany, 2024. Schloss Dagstuhl – Leibniz-Zentrum für Informatik.

Satoshi Nakamoto. Bitcoin: A peer-to-peer electronic cash system. Technical report, 2008.

Alex Nezlobin and Martin Tassy. Loss-versus-rebalancing under deterministic and generalized block-times, 2025. URL https://arxiv.org/abs/2505.05113.

Rithvik Rao and Nihar Shah. Triangle fees, 2023.

Ozan Solmaz, Lioba Heimbach, Yann Vonlanthen, and Roger Wattenhofer. Optimistic mev in ethereum layer 2s: Why blockspace is always in demand, 2025. URL https://arxiv.org/abs/2506.14768.

Martin Tassy and David White. Growth rate of a liquidity provider’s wealth in xy = c automated market makers, 2020.

## A. Proof of Lemma 2

Proof of Lemma 2. We consider Part (1), the others follow by analogy. Suppose the arbitrageur considers buying from the pool, and selling on the external market at price Pt. Then, the arbitrageur will face the optimization problem

$$
\begin{array} { r l } { \underset { \Delta x , \Delta y } { \mathrm { m a x i m i z e } } } & { P _ { t } \Delta x - e ^ { + \gamma _ { + } } \Delta y } \\ { \mathrm { s u b j e c t ~ t o } } & { f \left( x ^ { * } ( \tilde { P } _ { t ^ { - } } ) - \Delta x , y ^ { * } ( \tilde { P } _ { t ^ { - } } ) + \Delta y \right) = L , } \\ & { \Delta x , \Delta y \ge 0 , } \end{array}
$$

where $( x ^ { * } ( \tilde { P } _ { t ^ { - } } ) , y ^ { * } ( \tilde { P } _ { t ^ { - } } ) )$ are the reserves of the pool immediately prior to the arrival of the arbitrageur. Here, the decision variables $\Delta x$ describes the quantity of risky asset purchased by the arbitrageur, while $\Delta y$ is the amount of numéraire paid. Instead, we can parameterize the decision through the variables

$$
x \triangleq x ^ { \ast } ( \tilde { P } _ { t ^ { - } } ) - \Delta x , \quad y \triangleq y ^ { \ast } ( \tilde { P } _ { t ^ { - } } ) + \Delta y ,
$$

which describe the post-trade reserves of the pool. Thus, we can equivalently optimize

$$
\begin{array} { r l } { \underset { x , y } { \mathrm { m i n i m i z e } } } & { P _ { t } e ^ { - \gamma _ { + } } x + y } \\ { \mathrm { s u b j e c t ~ t o } } & { f \left( x , y \right) = L , } \\ & { x \leq x ^ { * } ( \tilde { P } _ { t ^ { - } } ) , \ y \geq y ^ { * } ( \tilde { P } _ { t ^ { - } } ) . } \end{array}\tag{28}
$$

Comparing to (3) and using the fact that $x ^ { * } ( \cdot )$ is monotonically decreasing while $y ^ { \ast } ( \cdot )$ is monotonically increasing, it is clear that the solution to (28) is given by

$$
x = \left\{ \begin{array} { l l l } { x ^ { * } \left( P _ { t } e ^ { - \gamma _ { + } } \right) } & { \mathrm { i f ~ } P _ { t } e ^ { - \gamma _ { + } } > \tilde { P } _ { t ^ { - } } , } \\ { x ^ { * } \left( \tilde { P } _ { t ^ { - } } \right) } & { \mathrm { o t h e r w i s e } , } \end{array} \right. \quad y = \left\{ \begin{array} { l l l } { y ^ { * } \left( P _ { t } e ^ { - \gamma _ { + } } \right) } & { \mathrm { i f ~ } P _ { t } e ^ { - \gamma _ { + } } > \tilde { P } _ { t ^ { - } } , } \\ { y ^ { * } \left( \tilde { P } _ { t ^ { - } } \right) } & { \mathrm { o t h e r w i s e } . } \end{array} \right.
$$

Therefore a profitable trade where the arbitrageur purchases from the pool is only possible when $P _ { t } > \tilde { P } _ { t ^ { - } } e ^ { + \gamma _ { + } }$ , and the profit is as given in Part (1). ■

## B. Proof of Theorem 1

Define the infinitesimal generator A by

$$
A f ( z ) \triangleq \operatorname* { l i m } _ { \Delta t  0 } \frac { 1 } { \Delta t } \mathsf { E } [ f ( z _ { \Delta t } ) - f ( z _ { 0 } ) | z _ { 0 } = z ] ,
$$

for $f : \mathbb { R } \to \mathbb { R }$ that is twice continuously differentiable. Then, it is easy to verify that

$$
A f ( z ) = { \frac { \sigma ^ { 2 } } { 2 } } f ^ { \prime \prime } ( z ) + \lambda \left[ f ( + \gamma ) - f ( z ) \right] \mathbb { I } _ { \{ z > + \gamma \} } + \lambda \left[ f ( - \gamma ) - f ( z ) \right] \mathbb { I } _ { \{ z < - \gamma \} } .
$$

Lemma 3. The process $z _ { t }$ is ergodic with a unique invariant distribution $\pi ( \cdot )$ on $\mathbb { R }$ , and this distribution is symmetric around $z = 0$ ·

Proof. Consider the Lyapunov function $V ( z ) \triangleq z ^ { 2 }$ . Then,

$$
A V ( z ) = \sigma ^ { 2 } - \lambda \left[ z ^ { 2 } - \gamma ^ { 2 } \right] \mathbb { I } _ { \{ z \not \in ( - \gamma , + \gamma ) \} } \leq \sigma ^ { 2 } + \lambda \gamma ^ { 2 } - \lambda V ( z ) ,
$$

i.e., this function satisfies the Foster-Lyapunov negative drift condition of Theorem 6.1 of Meyn and Tweedie [1993]. Hence, the process is ergodic and a unique stationary distribution exists. This stationary distribution $\pi ( \cdot )$ must also be symmetric around $z = 0$ . If not, define $\tilde { \pi } ( C ) \triangleq$

$\pi \left( \{ - z : z \in C \} \right)$ , for any measurable set $C \subset \mathbb { R }$ . Since the dynamics (9) are symmetric around $z = 0$ by Assumption $2 , \tilde { \pi } ( \cdot )$ must also be an invariant distribution, contradicting uniqueness. ■

Proof of Theorem 1. The invariant distribution $\pi ( \cdot )$ must satisfy

$$
\mathsf E _ { \pi } [ \boldsymbol { A } f ( \boldsymbol { z } ) ] = \int _ { - \infty } ^ { + \infty } \boldsymbol { A } f ( \boldsymbol { z } ) \pi ( d \boldsymbol { z } ) = 0 ,\tag{29}
$$

for all test functions $f \colon  { \mathbb { R } } \to  { \mathbb { R } }$ . We will guess that $\pi ( \cdot )$ decomposes according to three different densities over the three regions, and compute the conditional density on each segment via Laplace transforms using (29).

Define, for $\alpha \in \mathbb { R }$ , the test function

$$
f _ { + } ( z ) = { \left\{ \begin{array} { l l } { e ^ { - \alpha ( z - \gamma ) } } & { { \mathrm { i f ~ } } z > + \gamma , } \\ { 1 - \alpha ( z - \gamma ) } & { { \mathrm { o t h e r w i s e . } } } \end{array} \right. }
$$

Then, from (29),

$$
\begin{array} { l } { { 0 = \mathsf { E } _ { \pi } [ A f _ { + } ( z ) ] \qquad } } \\ { { \mathrm { = } \frac { \sigma ^ { 2 } \alpha ^ { 2 } } { 2 } \mathsf { E } _ { \pi } \left[ e ^ { - \alpha ( z - \gamma ) } \mathbb { I } _ { \left\{ z > + \gamma \right\} } \right] + \lambda \mathsf { E } _ { \pi } \left[ \left( 1 - e ^ { - \alpha ( z - \gamma ) } \right) \mathbb { I } _ { \left\{ z > + \gamma \right\} } \right] + \lambda \alpha \mathsf { E } _ { \pi } \left[ ( z + \gamma ) \mathbb { I } _ { \left\{ z < - \gamma \right\} } \right] \qquad } } \\ { { \mathrm { = } \frac { \sigma ^ { 2 } \alpha ^ { 2 } } { 2 } \mathsf { E } _ { \pi } \left[ e ^ { - \alpha ( z - \gamma ) } \mathbb { I } _ { \left\{ z > + \gamma \right\} } \right] + \lambda \mathsf { E } _ { \pi } \left[ \left( 1 - e ^ { - \alpha ( z - \gamma ) } \right) \mathbb { I } _ { \left\{ z > + \gamma \right\} } \right] - \lambda \alpha \mathsf { E } _ { \pi } \left[ ( z - \gamma ) \mathbb { I } _ { \left\{ z > + \gamma \right\} } \right] , } } \end{array}
$$

where for the last step we use symmetry. Dividing by $\lambda \pi _ { + }$ and conditioning,

$$
0 = \left( \frac { \alpha ^ { 2 } \gamma ^ { 2 } } { \eta ^ { 2 } } - 1 \right) \mathsf { E } _ { \pi } \left[ e ^ { - \alpha ( z - \gamma ) } \enspace \middle | \enspace z > + \gamma \right] + 1 - \alpha \mathsf { E } _ { \pi } \left[ z - \gamma \enspace \middle | \enspace z > + \gamma \right] .
$$

Then,

$$
\mathsf { E } _ { \pi } \left[ e ^ { - \alpha ( z - \gamma ) } \mid z > + \gamma \right] = \frac { \alpha \mathsf { E } _ { \pi } \left[ z - \gamma \mid z > + \gamma \right] - 1 } { \alpha ^ { 2 } \gamma ^ { 2 } / \eta ^ { 2 } - 1 }
$$

The denominator of this Laplace transform has two real roots, $\pm \eta / \gamma$ . We can exclude the positive root since $\pi ( \cdot )$ is a probability distribution. Then, conditioned on $z > + \gamma , z - \gamma$ must be exponential with parameter $\eta / \gamma = \sqrt { 2 \lambda } / \sigma$ This establishes that $\pi ( \cdot )$ is exponential conditioned on $z > + \gamma$ ， and by symmetry, also conditioned on $z < - \gamma$ . Note that

$$
\mathsf { E } _ { \pi } \left[ z - \gamma \mid z > + \gamma \right] = \gamma / \eta .\tag{30}
$$

Next, consider the test function

$$
f _ { 0 } ( z ) = \left\{ \begin{array} { l l } { e ^ { - \alpha \gamma } - \alpha e ^ { - \alpha \gamma } ( z - \gamma ) } & { \mathrm { i f ~ } z > + \gamma , } \\ { e ^ { - \alpha z } } & { \mathrm { i f ~ } z \in [ - \gamma , + \gamma ] , } \\ { e ^ { \alpha \gamma } - \alpha e ^ { \alpha \gamma } ( z + \gamma ) } & { \mathrm { i f ~ } z < - \gamma . } \end{array} \right.
$$

Then, from (29),

$$
\begin{array} { r l } & { 0 = \mathsf { E } _ { \pi } [ A f _ { 0 } ( z ) ] } \\ & { \quad = \frac { \sigma ^ { 2 } \alpha ^ { 2 } } { 2 } \mathsf { E } _ { \pi } \left[ e ^ { - \alpha z } \mathbb { I } _ { \left\{ z \in [ - \gamma , + \gamma ] \right\} } \right] + \lambda \alpha e ^ { - \alpha \gamma } \mathsf { E } _ { \pi } \left[ ( z - \gamma ) \mathbb { I } _ { \left\{ z > + \gamma \right\} } \right] + \lambda \alpha e ^ { \alpha \gamma } \mathsf { E } _ { \pi } \left[ ( z + \gamma ) \mathbb { I } _ { \left\{ z < - \gamma \right\} } \right] } \\ & { \quad = \frac { \sigma ^ { 2 } \alpha ^ { 2 } } { 2 } \mathsf { E } _ { \pi } \left[ e ^ { - \alpha z } \mathbb { I } _ { \left\{ z \in [ - \gamma , + \gamma ] \right\} } \right] + \lambda \alpha \left( e ^ { - \alpha \gamma } - e ^ { + \alpha \gamma } \right) \mathsf { E } _ { \pi } \left[ ( z - \gamma ) \mathbb { I } _ { \left\{ z > + \gamma \right\} } \right] , } \end{array}
$$

where for the last step we use symmetry. Dividing by $\lambda \pi _ { 0 }$ , conditioning, and using (30),

$$
0 = \frac { \alpha ^ { 2 } \gamma ^ { 2 } } { \eta ^ { 2 } } \mathsf E _ { \pi } \left[ e ^ { - \alpha z } \mid z \in \left[ - \gamma , + \gamma \right] \right] + \alpha \gamma \frac { e ^ { - \alpha \gamma } - e ^ { + \alpha \gamma } } \eta \frac { \pi _ { + } } { \pi _ { 0 } } .
$$

Rearranging,

$$
\mathsf { E } _ { \pi } \left[ e ^ { - \alpha z } \mid z \in [ - \gamma , + \gamma ] \right] = \frac { \eta } { \gamma } \frac { e ^ { + \alpha \gamma } - e ^ { - \alpha \gamma } } { \alpha } \frac { \pi _ { + } } { \pi _ { 0 } } .
$$

Inverting this Laplace transform, conditioned on $z \in [ - \gamma , + \gamma ] , \pi ( \cdot )$ is the uniform distribution. Moreover, we must have

$$
1 = \operatorname* { l i m } _ { \alpha  0 } \mathsf { E } _ { \pi } [ e ^ { - \alpha z } \big | z \in [ - \gamma , + \gamma ] ] = 2 \eta \pi _ { + } / \pi _ { 0 } ,
$$

so that $\pi _ { 0 } / \pi _ { + } = 2 \eta$ . Combining with the fact that $\pi _ { 0 } + 2 \pi _ { + } = 1$ , the result follows.

## C. Non-Symmetric Analysis

In this section, we consider dropping Assumption 2. The central implication of Assumption 2 is that the log-price process $z _ { t }$ is a driftless Brownian motion. In the absence of Assumption 2, $z _ { t }$ is a Brownian motion with drift, and a separate analysis is required for the stationary distribution. This is analogous to the two cases for stationary distribution of reflected Brownian motion [e.g., Prop. 6.6, Harrison, 2013]. In this section, we will establish the stationary distribution in the nonsymmetric case with drift. Once this result is established, the balance of the results in the paper can be derived as in the symmetric case.

In what follows, we will assume that the drift of the mispricing process with dynamics (7)–(9) is non-zero, i.e.,

$$
\Delta \triangleq \mu - { \textstyle { \frac { 1 } { 2 } } } \sigma ^ { 2 } \neq 0 .
$$

Here, the generator takes the form

$$
\begin{array} { r } { A f ( z ) = \Delta f ^ { \prime } ( z ) + \frac { 1 } { 2 } \sigma ^ { 2 } f ^ { \prime \prime } ( z ) + \lambda \left[ f ( + \gamma _ { + } ) - f ( z ) \right] \mathbb { I } _ { \{ z > + \gamma _ { + } \} } + \lambda \left[ f ( - \gamma _ { - } ) - f ( z ) \right] \mathbb { I } _ { \{ z < - \gamma _ { - } \} } , } \end{array}
$$

Theorem 8. The process $z _ { t }$ is an ergodic process on $\mathbb { R }$ , with unique invariant distribution $\pi ( \cdot )$ given by the density

$$
p _ { \pi } ( z ) = \left\{ \begin{array} { l l } { \pi _ { + } \times p _ { \zeta _ { + } } ^ { \mathrm { e x p } } ( z - \gamma _ { + } ) } & { i f z > + \gamma _ { + } , } \\ { \pi _ { 0 } \times \frac { \zeta _ { 0 } e ^ { - \zeta _ { 0 } x } } { e ^ { + \zeta _ { 0 } \gamma _ { - } } - e ^ { - \zeta _ { 0 } \gamma _ { + } } } } & { i f z \in [ - \gamma _ { - } , + \gamma _ { + } ] , } \\ { \pi _ { - } \times p _ { \zeta _ { - } } ^ { \mathrm { e x p } } ( - \gamma _ { - } - z ) } & { i f z < - \gamma _ { - } , } \end{array} \right.
$$

$f o r \ z \in \mathbb { R }$ . Here, the parameters are given by

$$
\zeta _ { + } \triangleq \frac { \sqrt { \Delta ^ { 2 } + 2 \lambda \sigma ^ { 2 } } - \Delta } { \sigma ^ { 2 } } > 0 , \quad \zeta _ { 0 } \triangleq \frac { 2 \Delta } { \sigma ^ { 2 } } , \quad \zeta _ { - } \triangleq \frac { \sqrt { \Delta ^ { 2 } + 2 \lambda \sigma ^ { 2 } } + \Delta } { \sigma ^ { 2 } } > 0 .
$$

The probabilities $\pi _ { - } , \pi _ { 0 } , \pi _ { + }$ of the three segments are given by

$$
\pi _ { 0 } \triangleq \left\{ 1 + \zeta _ { 0 } \left[ \frac { 1 } { \zeta _ { + } } \cdot \frac { 1 } { 1 - e ^ { - \zeta _ { 0 } ( \gamma _ { + } + \gamma _ { - } ) } } + \frac { 1 } { \zeta _ { - } } \cdot \left( \frac { 1 } { 1 - e ^ { - \zeta _ { 0 } ( \gamma _ { + } + \gamma _ { - } ) } } - 1 \right) \right] \right\} ^ { - 1 } ,
$$

$$
\pi _ { + } \triangleq \left\{ 1 + \zeta _ { + } \cdot \frac { \sigma ^ { 2 } } { 2 \Delta } + \zeta _ { + } \left( \frac { 1 } { \zeta _ { - } } - \frac { \sigma ^ { 2 } } { 2 \Delta } \right) e ^ { - \zeta _ { 0 } ( \gamma _ { + } + \gamma _ { - } ) } \right\} ^ { - 1 } ,
$$

$$
\pi _ { - } \triangleq \left\{ 1 + \zeta _ { - } \left[ \frac { 1 } { \zeta _ { + } } + \frac { \sigma ^ { 2 } } { 2 \Delta } \left\{ 1 - e ^ { - \zeta _ { 0 } ( \gamma _ { + } + \gamma _ { - } ) } \right\} \right] e ^ { \zeta _ { 0 } ( \gamma _ { + } + \gamma _ { - } ) + \gamma _ { - } ) } \right\} ^ { - 1 } .
$$

Final ly, $p _ { \zeta } ( x ) \triangleq \zeta e ^ { - \zeta x }$ is the density of an exponential distribution over $x \geq 0$ with parameter $\zeta$

Proof. The proof follows that of Theorem 1.

Upper test function:

$$
f _ { + } ( z ) = { \left\{ \begin{array} { l l } { e ^ { - \alpha ( z - \gamma _ { + } ) } } & { { \mathrm { i f ~ } } z > \gamma _ { + } , } \\ { 1 - \alpha ( z - \gamma _ { + } ) } & { { \mathrm { o t h e r w i s e . } } } \end{array} \right. }
$$

$$
\begin{array} { r l } & { 0 = \mathsf { E } _ { \pi } [ A f _ { + } ( z ) ] } \\ & { \quad = \alpha \left( \frac { 1 } { 2 } \sigma ^ { 2 } \alpha - \Delta \right) \mathsf { E } _ { \pi } \left[ e ^ { - \alpha \left( z - \gamma _ { + } \right) } \mathbb { I } _ { \left\{ z > \gamma _ { + } \right\} } \right] - \Delta \alpha \left( \pi _ { 0 } + \pi _ { - } \right) } \\ & { \quad \quad + \lambda \mathsf { E } _ { \pi } \left[ \left( 1 - e ^ { - \alpha \left( z - \gamma _ { + } \right) } \right) \mathbb { I } _ { \left\{ z > \gamma _ { + } \right\} } \right] + \lambda \alpha \mathsf { E } _ { \pi } \left[ \left( z - \gamma _ { - } \right) \mathbb { I } _ { \left\{ z < \gamma _ { - } \right\} } \right] } \end{array}
$$

Dividing by $\pi _ { + }$ and conditioning,

$$
\begin{array} { l } { { 0 = \alpha \left( { \frac { 1 } { 2 } } \sigma ^ { 2 } \alpha - \Delta \right) \mathsf E _ { \pi } \left[ e ^ { - \alpha \left( z - \gamma _ { + } \right) } \Big | z > \gamma _ { + } \right] - \Delta \alpha { \frac { \pi _ { 0 } + \pi _ { - } } { \pi _ { + } } } } } \\ { { \quad \ + \lambda \mathsf E _ { \pi } \left[ \left( 1 - e ^ { - \alpha \left( z - \gamma _ { + } \right) } \right) \Big | z > \gamma _ { + } \right] + \lambda \alpha \mathsf E _ { \pi } \left[ z - \gamma _ { - } | z < \gamma _ { - } \right] \frac { \pi _ { - } } { \pi _ { + } } } } \\ { { \ = \left\{ \alpha \left( { \frac { 1 } { 2 } } \sigma ^ { 2 } \alpha - \Delta \right) - \lambda \right\} \mathsf E _ { \pi } \left[ e ^ { - \alpha \left( z - \gamma _ { + } \right) } \Big | z > \gamma _ { + } \right] - \Delta \alpha \frac { \pi _ { 0 } + \pi _ { - } } { \pi _ { + } } } } \\ { { \ \ \qquad + \lambda + \lambda \alpha \mathsf E _ { \pi } \left[ z - \gamma _ { - } \big | z < \gamma _ { - } \right] \frac { \pi _ { - } } { \pi _ { + } } } } \end{array}
$$

Rearranging,

$$
\mathsf { E } _ { \pi } [ e ^ { - \alpha ( z - \gamma _ { + } ) } \Big | z > \gamma _ { + } ] = \frac { \Delta \alpha \frac { \pi _ { 0 } + \pi _ { - } } { \pi _ { + } } - \lambda + \lambda \alpha \mathsf { E } _ { \pi } [ \gamma _ { - } - z ] z < \gamma _ { - } ] \frac { \pi _ { - } } { \pi _ { + } } } { \frac { 1 } { 2 } \sigma ^ { 2 } \alpha ^ { 2 } - \Delta \alpha - \lambda }
$$

The denominator has two real roots, only one of which is negative. Then, the conditional distribution of $z - \gamma _ { + }$ must be exponential, with parameter

$$
\zeta _ { + } = \frac { 1 } { \sigma ^ { 2 } } \left( \sqrt { \Delta ^ { 2 } + 2 \lambda \sigma ^ { 2 } } - \Delta \right) > 0 .
$$

Additionally, note that

$$
\mathsf { E } _ { \pi } \left[ z - \gamma _ { + } | z > \gamma _ { + } \right] = \frac { 1 } { \zeta _ { + } } .\tag{31}
$$

Lower test function:

$$
f _ { - } ( z ) = { \left\{ \begin{array} { l l } { e ^ { - \alpha ( \gamma _ { - } - z ) } } & { { \mathrm { i f ~ } } z < \gamma _ { - } , } \\ { 1 + \alpha ( z - \gamma _ { - } ) } & { { \mathrm { o t h e r w i s e . } } } \end{array} \right. }
$$

By analogous arguments to the above, we have that

$$
\mathsf { E } _ { \pi } \left[ e ^ { - \alpha ( \gamma _ { -- } z ) } \Big | z < \gamma _ { - } \right] = \frac { - \Delta \alpha \frac { \pi _ { 0 } + \pi _ { + } } { \pi _ { - } } - \lambda + \lambda \alpha \mathsf { E } _ { \pi } \left[ z - \gamma _ { + } \big | z > \gamma _ { + } \right] \frac { \pi _ { + } } { \pi _ { - } } } { \frac { 1 } { 2 } \sigma ^ { 2 } \alpha ^ { 2 } + \Delta \alpha - \lambda } ,
$$

and therefore, the distribution of $\gamma _ { - } - z$ , conditioned on $z < \gamma _ { - }$ , is exponential with parameter

$$
\zeta _ { - } = \frac { 1 } { \sigma ^ { 2 } } \left( \sqrt { \Delta ^ { 2 } + 2 \lambda \sigma ^ { 2 } } + \Delta \right) > 0 .
$$

Similarly, note that

$$
\mathsf { E } _ { \pi } [ \gamma _ { - } - z | z < \gamma _ { - } ] = \frac { 1 } { \zeta _ { - } } .\tag{32}
$$

Middle test function:

$$
f _ { 0 } ( z ) = \left\{ \begin{array} { l l } { e ^ { - \alpha \gamma _ { + } } - \alpha e ^ { - \alpha \gamma _ { + } } ( z - \gamma _ { + } ) } & { \mathrm { i f ~ } z > \gamma _ { + } , } \\ { e ^ { - \alpha z } } & { \mathrm { i f ~ } z \in [ \gamma _ { - } , \gamma _ { + } ] , } \\ { e ^ { - \alpha \gamma _ { - } } - \alpha e ^ { - \alpha \gamma _ { - } } ( z - \gamma _ { - } ) } & { \mathrm { i f ~ } z < \gamma _ { - } . } \end{array} \right.
$$

$$
\begin{array} { r l } {  { 0 = \mathsf { E } _ { \pi } \bigl [ A f _ { 0 } ( z ) \bigr ] } } \\ & { = \alpha ( \frac { 1 } { 2 } \sigma ^ { 2 } \alpha - \Delta ) \mathsf { E } _ { \pi } [ e ^ { - \alpha z } \mathbb { I } _ { \{ z \in [ \gamma _ { - } , \gamma _ { + } ] \} } ] } \\ & { \phantom { = } - \Delta \alpha ( e ^ { - \alpha \gamma _ { + } } \pi _ { + } + e ^ { - \alpha \gamma _ { - } } \pi _ { - } ) } \\ & { \phantom { = } + \lambda \alpha e ^ { - \alpha \gamma _ { + } } \mathsf { E } _ { \pi } [ ( z - \gamma _ { + } ) \mathbb { I } _ { \{ z > \gamma _ { + } \} } ] + \lambda \alpha e ^ { - \alpha \gamma _ { - } } \mathsf { E } _ { \pi } [ ( z - \gamma _ { - } ) \mathbb { I } _ { \{ z < \gamma _ { - } \} } ] . } \end{array}
$$

Dividing by $\pi _ { 0 }$ and conditioning,

$$
\begin{array} { r l } & { 0 = \alpha \left( \frac { 1 } { 2 } \sigma ^ { 2 } \alpha - \Delta \right) \mathsf { E } _ { \pi } \left[ e ^ { - \alpha z } | z \in \left[ \gamma _ { - } , \gamma _ { + } \right] \right] } \\ & { \quad \quad - \Delta \alpha \left( e ^ { - \alpha \gamma _ { + } } \frac { \pi _ { + } } { \pi _ { 0 } } + e ^ { - \alpha \gamma _ { - } } \frac { \pi _ { - } } { \pi _ { 0 } } \right) } \\ & { \quad \quad + \lambda \alpha \left( e ^ { - \alpha \gamma _ { + } } \mathsf { E } _ { \pi } \left[ z - \gamma _ { + } | z > \gamma _ { + } \right] \frac { \pi _ { + } } { \pi _ { 0 } } - e ^ { - \alpha \gamma _ { - } } \mathsf { E } _ { \pi } \left[ \gamma _ { - } - z | z < \gamma _ { - } \right] \frac { \pi _ { - } } { \pi _ { 0 } } \right) . } \end{array}
$$

Rearranging, and using (31) and (32),

$$
\begin{array} { r l } & { \mathsf { E } _ { \pi } \left[ e ^ { - \alpha z } | z \in [ \gamma _ { - } , \gamma _ { 1 } ] \right] = \frac { \Delta \left( e ^ { - \alpha \gamma _ { + } } \frac { \pi + } { \pi _ { 0 } } + e ^ { - \alpha \gamma _ { - } } \frac { \pi } { \pi _ { 0 } } \right) - \lambda \left( e ^ { - \alpha \gamma _ { + } } \mathsf { E } _ { \pi } \left[ z - \gamma _ { + } | z > \gamma _ { \downarrow } \right] \frac { \pi _ { + } } { \pi _ { 0 } } - e ^ { - \alpha \gamma _ { - } } \mathsf { E } _ { \pi } \left[ \gamma _ { - } - z \right| z < \gamma _ { - } \right] \frac { \pi } { \pi } } { \frac { 1 } { \pi _ { 0 } } } } \\ & { \phantom { \frac { E _ { \pi } ^ { 2 } } { = } } = \frac { e ^ { - \alpha \gamma _ { + } } \left( \Delta - \frac { \lambda } { \xi _ { + } } \right) \frac { \pi _ { + } } { \pi _ { 0 } } + e ^ { - \alpha \gamma _ { - } } \left( \Delta + \frac { \lambda } { \xi _ { - } } \right) \frac { \pi } { \pi _ { 0 } } } { \frac { 1 } { 2 } e ^ { 2 } \alpha - \Delta } } \\ & { \phantom { \frac { E _ { \pi } ^ { 2 } } { = } } = \frac { - \zeta _ { + } \cdot \frac { \pi _ { + } } { \pi _ { 0 } } e ^ { - \alpha \gamma _ { + } } + \zeta _ { - } \cdot \frac { \pi _ { - } } { \pi _ { 0 } } e ^ { - \alpha \gamma _ { - } } } { \alpha - \zeta _ { 0 } } } \end{array}
$$

Inverting this Laplace transform, conditioned on $z \in [ \gamma _ { - } , \gamma _ { + } ] , \pi ( \cdot )$ is the superposition of two appropriately-centered truncated exponential distributions. Moreover, we must have

$$
1 = \operatorname* { l i m } _ { \alpha  0 } \mathsf { E } _ { \pi } [ e ^ { - \alpha z } | z \in [ \gamma _ { - } , \gamma _ { + } ] ] = \frac { \zeta _ { + } \cdot \frac { \pi _ { + } } { \pi _ { 0 } } - \zeta _ { - } \cdot \frac { \pi _ { - } } { \pi _ { 0 } } } { \zeta _ { 0 } } ,
$$

and additionally, since the Laplace transform corresponds to the conditional density for $z \in [ \gamma _ { - } , \gamma _ { + } ]$ the density

$$
\begin{array} { c } { { \zeta _ { + } \cdot \frac { \pi _ { + } } { \pi _ { 0 } } \left[ \exp \left( \zeta _ { 0 } ( z - \gamma _ { - } ) \right) u ( z - \gamma _ { - } ) - \exp \left( \zeta _ { 0 } ( z - \gamma _ { + } ) \right) u ( z - \gamma _ { + } ) \right] } } \\ { { - \zeta _ { 0 } \exp \left( \zeta _ { 0 } ( z - \gamma _ { - } ) \right) u ( z - \gamma _ { - } ) } } \end{array}
$$

must be zero for $z > \gamma _ { + }$ , yielding the equation (only if $\mu \neq \sigma ^ { 2 } / 2 )$

$$
\zeta _ { + } \cdot \frac { \pi _ { + } } { \pi _ { 0 } } = \left( \zeta _ { 0 } \right) \bigg / \left( 1 - \exp \left( - \zeta _ { 0 } ( \gamma _ { + } - \gamma _ { - } ) \right) \right) .
$$

Finally, solving the linear system of equations, combining with the fact that $\pi _ { 0 } + \pi _ { + } + \pi _ { - } = 1$

yields the result (only if $\mu \neq \sigma ^ { 2 } / 2 )$

$$
\pi _ { 0 } = 1 \Bigg / \left\{ 1 + \zeta _ { 0 } \cdot \left[ \frac { 1 } { \zeta _ { + } } \cdot \frac { 1 } { 1 - \exp { ( - \zeta _ { 0 } ( \gamma _ { + } - \gamma _ { - } ) ) } } + \frac { 1 } { \zeta _ { - } } \cdot \left( \frac { 1 } { 1 - \exp { ( - \zeta _ { 0 } ( \gamma _ { + } - \gamma _ { - } ) ) } } - 1 \right) \right] \right\}
$$

$$
\pi _ { + } = 1 \Bigg / \left\{ 1 + \zeta _ { + } \cdot \frac { \sigma ^ { 2 } } { 2 \Delta } + \zeta _ { + } \left( \frac { 1 } { \zeta _ { - } } - \frac { \sigma ^ { 2 } } { 2 \Delta } \right) \exp \left( - \zeta _ { 0 } ( \gamma _ { + } - \gamma _ { - } ) \right) \right\}
$$

$$
\pi _ { - } = 1 \left/ \left\{ 1 + \zeta _ { - } \left[ \frac { 1 } { \zeta _ { + } } + \frac { \sigma ^ { 2 } } { 2 \Delta } \left\{ 1 - \exp \left( - \zeta _ { 0 } ( \gamma _ { + } - \gamma _ { - } ) \right) \right\} \right] \exp \left( \zeta _ { 0 } ( \gamma _ { + } - \gamma _ { - } ) - \gamma _ { - } \right) \right) \right\} .
$$

## D. Proof of Corollary 2

Proof of Corollary 2. For this pool, we have that

$$
V ( P ) = 2 L \sqrt { P } , \quad x ^ { * } ( P ) = L / \sqrt { P } , \quad y ^ { * } ( P ) = L \sqrt { P } .
$$

Following from Theorem 2,

$$
\overline { { \frac { \mathsf { A R B } } { V ( P ) } } } = \lambda \mathsf E _ { \pi } \biggl [ \frac { A _ { + } ( P , z ) + A _ { - } ( P , z ) } { V ( P ) } \biggr ] .\tag{33}
$$

Note that, in this case,

$$
\begin{array} { l } { { \displaystyle \frac { A _ { + } ( P , z ) } { V ( P ) } = \frac { 1 } { 2 L \sqrt { P } } \left[ P \left\{ x ^ { * } \left( P e ^ { - z } \right) - x ^ { * } \left( P e ^ { - \gamma } \right) \right\} + e ^ { + \gamma } \left\{ y ^ { * } \left( P e ^ { - z } \right) - y ^ { * } \left( P e ^ { - \gamma } \right) \right\} \right] \mathbb { I } _ { \left\{ z > + \gamma \right\} } } } \\ { { \displaystyle \qquad = \frac { 1 } { 2 } \left[ \left\{ e ^ { + z / 2 } - e ^ { + \gamma / 2 } \right\} + e ^ { + \gamma } \left\{ e ^ { - z / 2 } - e ^ { - \gamma / 2 } \right\} \right] \mathbb { I } _ { \left\{ z > + \gamma \right\} } } } \\ { { \displaystyle \qquad = \frac { 1 } { 2 } e ^ { + \gamma / 2 } \left[ e ^ { + ( z - \gamma ) / 2 } - 2 + e ^ { - ( z - \gamma ) / 2 } \right] \mathbb { I } _ { \left\{ z > + \gamma \right\} } . } } \end{array}
$$

Taking a conditional expectation over $z > + \gamma$

$$
\begin{array} { r l } & { \mathbb { E } _ { \boldsymbol \pi } \left[ \frac { A _ { + } ( P , z ) } { V ( P ) } \middle | z > + \gamma \right] = \left\{ \begin{array} { l l } { \frac { 1 } { 2 } e ^ { + \gamma / 2 } \left[ \frac { \eta / \gamma } { \eta / \gamma - 1 / 2 } - 2 + \frac { \eta / \gamma } { \eta / \gamma + 1 / 2 } \right] } & { \mathrm { i f ~ } 1 / 2 < \eta / \gamma , } \\ { + \infty } & { \mathrm { o t h e r w i s e } , } \end{array} \right. } \\ & { \qquad = \left\{ \begin{array} { l l } { \frac { 1 } { 2 } e ^ { + \gamma / 2 } \left[ \frac { \sqrt { 2 \lambda } / \sigma } { \sqrt { 2 \lambda / \sigma - 1 / 2 } } - 2 + \frac { \sqrt { 2 \lambda } / \sigma } { \sqrt { 2 \lambda / \sigma + 1 / 2 } } \right] } & { \mathrm { i f ~ } \sigma / \sqrt { 2 \lambda } < 2 , } \\ { + \infty } & { \mathrm { o t h e r w i s e } , } \end{array} \right. } \\ & { \qquad = \left\{ \begin{array} { l l } { \frac { e ^ { + \gamma / 2 } } { 8 \lambda / \sigma ^ { 2 } - 1 } } & { \mathrm { i f ~ } \sigma ^ { 2 } / 8 < \lambda , } \\ { + \infty } & { \mathrm { o t h e r w i s e } . } \end{array} \right. } \end{array}
$$

For the remainder of the proof, assume that $\sigma ^ { 2 } / 8 < \lambda$ . Taking an unconditional expectation and multiplying by λ,

$$
\begin{array} { r l r } & { } & { \lambda \mathsf { E } _ { \pi } \left[ \frac { A _ { + } ( P , z ) } { V ( P ) } \right] = \pi _ { + } \times \lambda \mathsf { E } _ { \pi } \left[ \frac { A _ { + } ( P , z ) } { V ( P ) } \Big | z > + \gamma \right] } \\ & { } & { = \frac { \sigma ^ { 2 } } { 8 } \times \mathsf { P } _ { \mathrm { t r a d e } } \times \frac { e ^ { + \gamma / 2 } } { 2 \left( 1 - \sigma ^ { 2 } / ( 8 \lambda ) \right) } . } \end{array}
$$

Combining with the symmetric case for $A _ { - } ( P , z ) / V ( P )$ , and applying (33), the result follows.

Now, we consider fees. Following from Theorem 2,

$$
\overline { { \mathsf { n F E E } } } = \lambda \mathsf E _ { \pi } \left[ \frac { F _ { + } ( P , z ) + F _ { - } ( P , z ) } { V ( P ) } \right] .\tag{34}
$$

Then,

$$
\begin{array} { l } { \displaystyle \frac { F _ { + } ( P , z ) } { V ( P ) } = - \frac { 1 } { 2 L \sqrt { P } } \big ( e ^ { + \gamma } - 1 \big ) \left[ y ^ { * } \left( P e ^ { - z } \right) - y ^ { * } \left( P e ^ { - \gamma } \right) \right] \mathbb { I } _ { \{ z > + \gamma \} } } \\ { \displaystyle \quad = - \frac { 1 } { 2 } \Big ( e ^ { + \gamma } - 1 \Big ) \left[ e ^ { - z / 2 } - e ^ { - \gamma / 2 } \right] \mathbb { I } _ { \{ z > + \gamma \} } } \\ { \displaystyle \quad = \frac { e ^ { + \gamma / 2 } - e ^ { - \gamma / 2 } } { 2 } \left[ 1 - e ^ { - ( z - \gamma ) / 2 } \right] \mathbb { I } _ { \{ z > + \gamma \} } . } \end{array}
$$

Taking a conditional expectation over z,

$$
\begin{array} { r } { \mathsf { E } _ { \pi } \left[ \left. \frac { F _ { + } ( P , z ) } { V ( P ) } \right| z > + \gamma \right] = \frac { e ^ { + \gamma / 2 } - e ^ { - \gamma / 2 } } { 2 } \left[ 1 - \frac { \eta / \gamma } { \eta / \gamma + 1 / 2 } \right] } \\ { = \frac { e ^ { + \gamma / 2 } - e ^ { - \gamma / 2 } } { 4 } \times \frac { 1 } { \sqrt { 2 \lambda } / \sigma + 1 / 2 } } \end{array}
$$

Taking an unconditional expectation,

$$
\begin{array} { c } { { \displaystyle \mathsf { E } _ { \boldsymbol \pi } \left[ \frac { F _ { + } ( P , z ) } { V ( P ) } \right] = \pi ( z > + \gamma ) \times \mathsf { E } _ { \boldsymbol \pi } \left[ \frac { F _ { + } ( P , z ) } { V ( P ) } \Big | z > + \gamma \right] } } \\ { { = \displaystyle \frac { e ^ { + \gamma / 2 } - e ^ { - \gamma / 2 } } { 4 } \times \frac { 1 } { \left( \sqrt { 2 \lambda } \gamma / \sigma + 1 \right) \left( \sqrt { 2 \lambda } / \sigma + 1 / 2 \right) } } } \\ { { = \displaystyle \frac { e ^ { + \gamma / 2 } - e ^ { - \gamma / 2 } } { 4 \gamma } \times \frac { \sigma ^ { 2 } } { 4 \lambda } \times \frac { 1 } { \left( 1 + \sigma / \left( \sqrt { 2 \lambda } \gamma \right) \right) \left( 1 + \sigma / \left( 2 \sqrt { 2 \lambda } \right) \right) } . } } \end{array}
$$

Combining with the symmetric case for $F _ { - } ( P , z ) / V ( P )$ , and applying (34),

$$
\overline { { \mathfrak { n } \mathsf { F } \mathsf { E } \mathsf { E } } } = \frac { \sigma ^ { 2 } } { 8 } \times \frac { e ^ { + \gamma / 2 } - e ^ { - \gamma / 2 } } { \gamma } \times \frac { 1 } { \left( 1 + \sigma / \left( \sqrt { 2 \lambda } \gamma \right) \right) \left( 1 + \sigma / \left( 2 \sqrt { 2 \lambda } \right) \right) } .
$$

## E. Proof of Theorems 3 and 4

Proof of Theorem 3. Fix $P > 0$ . Note that, from the definitions of $A _ { + } ( P , \cdot )$ and $A _ { - } ( P , \cdot )$ , it is easy to see that

$$
\bar { A } ( P , 0 ) = 0 , \qquad \bar { A } ( P , x ) \geq 0 , \forall x \geq 0 ,\tag{35}
$$

$$
\partial _ { x } \bar { A } ( P , 0 ) = 0 , \partial _ { x } \bar { A } ( P , x ) \geq 0 , \forall x \geq 0 ,\tag{36}
$$

$$
\partial _ { x x } \bar { A } ( P , 0 ) = P { \frac { y ^ { * ^ { \prime } } ( P e ^ { - \gamma } ) + e ^ { + \gamma } \cdot y ^ { * ^ { \prime } } ( P e ^ { + \gamma } ) } { 2 } } .\tag{37}
$$

Define the Laplace transform

$$
F ( s ) = \int _ { 0 } ^ { \infty } { \bar { A } } ( P , x ) e ^ { - s x } d x ,\tag{38}
$$

for $s \in \mathbb { R }$ . Observe that, from (10),

$$
{ \overline { { \mathsf { A R B } } } } = \lambda { \mathsf { P } } _ { \mathrm { t r a d e } } { \frac { \sqrt { 2 \lambda } } { \sigma } } F \left( { \frac { \sqrt { 2 \lambda } } { \sigma } } \right) .\tag{39}
$$

Applying the derivative formula for Laplace transforms (integration-by-parts) twice to (38), and using (35)–(36),

$$
s F ( s ) = \underbrace { \bar { A } ( P , 0 ) } _ { = 0 } + \int _ { 0 } ^ { \infty } e ^ { - s x } \partial _ { x } \bar { A } ( P , x ) d x ,
$$

$$
s ^ { 2 } F ( s ) = \underbrace { \partial _ { x } { \bar { A } } ( P , 0 ) } _ { = 0 } + \int _ { 0 } ^ { \infty } e ^ { - s x } \partial _ { x x } { \bar { A } } ( P , x ) d x .
$$

Observe that $s ^ { 2 } F ( s )$ is the Laplace transform of the function $\partial _ { x x } { \bar { A } } ( P , \cdot )$ . Then, applying the initial value theorem for Laplace transforms15 and (37),

$$
\operatorname* { l i m } _ { s \to \infty } s \times s ^ { 2 } F ( s ) = \operatorname* { l i m } _ { x \to 0 } \partial _ { x x } { \bar { A } } ( P , x ) = P { \frac { y ^ { * / } \left( P e ^ { - \gamma } \right) + e ^ { + \gamma } \cdot y ^ { * / } \left( P e ^ { + \gamma } \right) } { 2 } } .
$$

Comparing with (39),

$$
P \frac { y ^ { \ast ^ { \prime } } ( P e ^ { - \gamma } ) + e ^ { + \gamma } \cdot y ^ { \ast ^ { \prime } } \left( P e ^ { + \gamma } \right) } { 2 } = \operatorname* { l i m } _ { \lambda \to \infty } \left( \frac { \sqrt { 2 \lambda } } { \sigma } \right) ^ { 3 } F \left( \frac { \sqrt { 2 \lambda } } { \sigma } \right) = \operatorname* { l i m } _ { \lambda \to \infty } \frac { \overline { { \mathrm { A R B } } } } { \sigma ^ { 2 } / 2 \times \mathrm { P _ { t r a d e } } } .
$$

The result follows.

Proof of Theorem 4. We will follow the proof of Theorem 3. Fix $P > 0$ . Note that, from the

definitions of $F _ { + } ( P , \cdot )$ and $F _ { - } ( P , \cdot )$ , it is easy to see that

$$
\bar { F } ( P , 0 ) = 0 , \qquad \bar { F } ( P , x ) \geq 0 , \ \forall x \geq 0 ,\tag{40}
$$

$$
\partial _ { x } \bar { F } ( P , 0 ) = P \frac { ( 1 - e ^ { - \gamma } ) y ^ { * \prime } ( P e ^ { - \gamma } ) + ( e ^ { + \gamma } - 1 ) y ^ { * \prime } ( P e ^ { + \gamma } ) } { 2 } , \partial _ { x } \bar { F } ( P , x ) \geq 0 , \forall x \geq 0 .\tag{41}
$$

Define the Laplace transform

$$
G ( s ) = \int _ { 0 } ^ { \infty } { \bar { F } } ( P , x ) e ^ { - s x } d x ,\tag{42}
$$

for $s \in \mathbb { R }$ . Observe that, from (11),

$$
\overline { { { \mathsf { F E E } } } } = \lambda \mathsf { P } _ { \mathrm { t r a d e } } \frac { \sqrt { 2 \lambda } } { \sigma } G \left( \frac { \sqrt { 2 \lambda } } { \sigma } \right) .\tag{43}
$$

Applying the derivative formula for Laplace transforms (integration-by-parts) to (42), and using (40),

$$
s G ( s ) = \underbrace { { \bar { F } } ( P , 0 ) } _ { = 0 } + \int _ { 0 } ^ { \infty } e ^ { - s x } \partial _ { x } { \bar { F } } ( P , x ) d x .
$$

Observe that $s G ( s )$ is the Laplace transform of the function $\partial _ { x } { \bar { F } } ( P , \cdot )$ . Then, applying the initial value theorem for Laplace transforms16 and (41), we get that

$$
\operatorname* { l i m } _ { s \to \infty } s \times s G ( s ) = \operatorname* { l i m } _ { x \to 0 } \partial _ { x } { \bar { F } } ( P , x ) = P { \frac { ( 1 - e ^ { - \gamma } ) y ^ { * \prime } ( P e ^ { - \gamma } ) + ( e ^ { + \gamma } - 1 ) y ^ { * \prime } ( P e ^ { + \gamma } ) } { 2 } } .
$$

Comparing with (43),

$$
P \frac { ( 1 - e ^ { - \gamma } ) y ^ { * \prime } ( P e ^ { - \gamma } ) + ( e ^ { + \gamma } - 1 ) y ^ { * \prime } ( P e ^ { + \gamma } ) } { 2 \gamma } = \frac { 1 } { \gamma } \operatorname* { l i m } _ { \lambda  \infty } ( \frac { \sqrt { 2 \lambda } } { \sigma } ) ^ { 2 } G ( \frac { \sqrt { 2 \lambda } } { \sigma } )
$$

The result follows.

## F. Proof of Theorem 5

Define the infinitesimal generator A by

$$
A f ( z ) \triangleq \operatorname* { l i m } _ { \Delta t  0 } \frac { 1 } { \Delta t } \mathsf { E } [ f ( z _ { \Delta t } ) - f ( z _ { 0 } ) | z _ { 0 } = z ] ,
$$

for $f : \mathbb { R } \to \mathbb { R }$ that is twice continuously differentiable. Then, it is easy to verify that

$$
A f ( z ) = \frac { \sigma ^ { 2 } } { 2 } f ^ { \prime \prime } ( z ) + \lambda \left[ f ( + \gamma ) - f ( z ) \right] \mathbb { I } _ { \{ z > + \bar { \gamma } \} } + \lambda \left[ f ( - \gamma ) - f ( z ) \right] \mathbb { I } _ { \{ z < - \bar { \gamma } \} } .
$$

Lemma 4. The process $z _ { t }$ is ergodic with a unique invariant distribution $\pi ( \cdot )$ on $\mathbb { R }$ , and this distribution is symmetric around $z = 0$

Proof. Consider the Lyapunov function $V ( z ) \triangleq z ^ { 2 }$ . Then,

$$
A V ( z ) = \sigma ^ { 2 } - \lambda \left[ z ^ { 2 } - \gamma ^ { 2 } \right] \mathbb { I } _ { \{ z \not \in ( - \bar { \gamma } , + \bar { \gamma } ) \} } \leq \sigma ^ { 2 } + \lambda \gamma ^ { 2 } - \lambda V ( z ) ,
$$

i.e., this function satisfies the Foster-Lyapunov negative drift condition of Theorem 6.1 of Meyn and Tweedie [1993]. Hence, the process is ergodic and a unique stationary distribution exists. This stationary distribution $\pi ( \cdot )$ must also be symmetric around $z = 0$ . If not, define $\tilde { \pi } ( C ) \triangleq$ $\pi \left( \{ - z : z \in C \} \right)$ , for any measurable set $C \subset \mathbb { R }$ . Since the dynamics (9) are symmetric around $z = 0$ by Assumption $2 , \tilde { \pi } ( \cdot )$ must also be an invariant distribution, contradicting uniqueness.

Proof of Theorem 5. The invariant distribution $\pi ( \cdot )$ must satisfy

$$
\mathsf E _ { \pi } [ \boldsymbol { A } f ( \boldsymbol { z } ) ] = \int _ { - \infty } ^ { + \infty } \boldsymbol { A } f ( \boldsymbol { z } ) \pi ( d \boldsymbol { z } ) = 0 ,\tag{44}
$$

for all test functions $f \colon  { \mathbb { R } } \to  { \mathbb { R } }$ . We will guess that $\pi ( \cdot )$ decomposes according to three different densities over the three regions, and compute the conditional density on each segment via Laplace transforms using (44).

Define, for $\alpha \in \mathbb { R }$ , the test function

$$
f _ { + } ( z ) = { \left\{ \begin{array} { l l } { e ^ { - \alpha ( z - { \bar { \gamma } } ) } } & { { \mathrm { i f ~ } } z > + { \bar { \gamma } } , } \\ { 1 - \alpha ( z - { \bar { \gamma } } ) } & { { \mathrm { o t h e r w i s e . } } } \end{array} \right. }
$$

Then, from (44),

$$
\begin{array} { r l } & { 0 = \mathsf { E } _ { \pi } [ A f _ { + } ( z ) ] } \\ & { \quad = \frac { \sigma ^ { 2 } \alpha ^ { 2 } } { 2 } \mathsf { E } _ { \pi } \left[ e ^ { - \alpha ( z - \tilde { \gamma } ) } \mathbb { I } _ { \{ z > + \tilde { \gamma } \} } \right] + \lambda \mathsf { E } _ { \pi } \left[ \left( 1 + \alpha ( \bar { \gamma } - \gamma ) - e ^ { - \alpha ( z - \tilde { \gamma } ) } \right) \mathbb { I } _ { \{ z > + \tilde { \gamma } \} } \right] + \lambda \alpha \mathsf { E } _ { \pi } \left[ ( z + \gamma ) \mathbb { I } _ { \{ z < - \tilde { \gamma } \} } \right] } \\ & { \quad = \frac { \sigma ^ { 2 } \alpha ^ { 2 } } { 2 } \mathsf { E } _ { \pi } \left[ e ^ { - \alpha ( z - \tilde { \gamma } ) } \mathbb { I } _ { \{ z > + \tilde { \gamma } \} } \right] + \lambda \mathsf { E } _ { \pi } \left[ \left( 1 + \alpha ( \bar { \gamma } - \gamma ) - e ^ { - \alpha ( z - \tilde { \gamma } ) } \right) \mathbb { I } _ { \{ z > + \tilde { \gamma } \} } \right] - \lambda \alpha \mathsf { E } _ { \pi } \left[ ( z - \gamma ) \mathbb { I } _ { \{ z > + \tilde { \gamma } \} } \right] , } \end{array}
$$

where for the last step we use symmetry. Dividing by $\lambda \pi _ { + }$ and conditioning,17

$$
0 = \left( \frac { \alpha ^ { 2 } \bar { \gamma } ^ { 2 } } { \eta ^ { 2 } } - 1 \right) \mathsf { E } _ { \pi } \left[ e ^ { - \alpha ( z - \bar { \gamma } ) } \mid z > + \bar { \gamma } \right] + 1 + \alpha ( \bar { \gamma } - \gamma ) - \alpha \mathsf { E } _ { \pi } \left[ z - \gamma \mid z > + \bar { \gamma } \right] .
$$

Then,

$$
\mathsf { E } _ { \pi } \left[ e ^ { - \alpha ( z - \bar { \gamma } ) } \mid z > + \bar { \gamma } \right] = \frac { \alpha \mathsf { E } _ { \pi } \left[ z - \bar { \gamma } \mid z > + \bar { \gamma } \right] - 1 } { \alpha ^ { 2 } \bar { \gamma } ^ { 2 } / \eta ^ { 2 } - 1 }
$$

The denominator of this Laplace transform has two real roots, $\pm \eta / \bar { \gamma }$ . We can exclude the positive root since $\pi ( \cdot )$ is a probability distribution. Then, conditioned on $z > + \bar { \gamma } , z - \bar { \gamma }$ must be exponential with parameter $\eta / \bar { \gamma } = \sqrt { 2 \lambda } / \sigma$ . This establishes that $\pi ( \cdot )$ is exponential conditioned on $z > + \bar { \gamma }$ ， and by symmetry, also conditioned on $z < - \bar { \gamma }$ . Note that

$$
\mathsf { E } _ { \pi } \left[ z - \bar { \gamma } \mid z > + \bar { \gamma } \right] = \bar { \gamma } / \eta .\tag{45}
$$

Next, consider the test function

$$
f _ { 0 } ( z ) = \left\{ \begin{array} { l l } { e ^ { - \alpha \bar { \gamma } } - \alpha e ^ { - \alpha \bar { \gamma } } ( z - \bar { \gamma } ) } & { \mathrm { i f ~ } z > + \bar { \gamma } , } \\ { e ^ { - \alpha z } } & { \mathrm { i f ~ } z \in [ - \bar { \gamma } , + \bar { \gamma } ] , } \\ { e ^ { \alpha \bar { \gamma } } - \alpha e ^ { \alpha \bar { \gamma } } ( z + \bar { \gamma } ) } & { \mathrm { i f ~ } z < - \bar { \gamma } . } \end{array} \right.
$$

Then, from (44),

$$
\begin{array} { l } { { 0 = { \mathbb E } _ { \pi } [ A f _ { 0 } ( z ) ] } } \\ { { \ = { \frac { \sigma ^ { 2 } \alpha ^ { 2 } } { 2 } } { \mathbb E } _ { \pi } \left[ e ^ { - \alpha z } { \mathbb I } _ { \left\{ z \in [ - \bar { \gamma } , + \bar { \gamma } ] \right\} } \right] + \lambda \left( e ^ { - \alpha \gamma } - e ^ { - \alpha \bar { \gamma } } \right) \pi _ { + } + \lambda \alpha e ^ { - \alpha \bar { \gamma } } { \mathbb E } _ { \pi } \left[ ( z - \bar { \gamma } ) { \mathbb I } _ { \left\{ z > + \bar { \gamma } \right\} } \right] } } \\ { { \ + \lambda \left( e ^ { + \alpha \gamma } - e ^ { + \alpha \bar { \gamma } } \right) \pi _ { - } + \lambda \alpha e ^ { \alpha \bar { \gamma } } { \mathbb E } _ { \pi } \left[ ( z + \bar { \gamma } ) { \mathbb I } _ { \left\{ z < - \bar { \gamma } \right\} } \right] . } } \end{array}
$$

Dividing by $\lambda \pi _ { 0 }$ , conditioning, using symmetry, and using (45),

$$
0 = \frac { \alpha ^ { 2 } \bar { \gamma } ^ { 2 } } { \eta ^ { 2 } } \mathbb { E } _ { \pi } \left[ e ^ { - \alpha z } \middle | z \in [ - \bar { \gamma } , + \bar { \gamma } ] \right] + \left( \alpha \bar { \gamma } \frac { e ^ { - \alpha \bar { \gamma } } - e ^ { + \alpha \bar { \gamma } } } { \eta } + e ^ { - \alpha \gamma } + e ^ { + \alpha \gamma } - e ^ { + \alpha \bar { \gamma } } - e ^ { - \alpha \bar { \gamma } } \right) \cdot \frac { \pi _ { + } } { \pi _ { 0 } } .
$$

Rearranging,

$$
\mathsf E _ { \pi } \left[ e ^ { - \alpha z } \mid z \in [ - \bar { \gamma } , + \bar { \gamma } ] \right] = \frac { \pi _ { + } } { \pi _ { 0 } } \left( \frac { \eta } { \bar { \gamma } } \cdot \frac { e ^ { + \alpha \bar { \gamma } } - e ^ { - \alpha \bar { \gamma } } } { \alpha } + \frac { e ^ { \alpha \bar { \gamma } } + e ^ { - \alpha \bar { \gamma } } } { \alpha ^ { 2 } } \cdot \frac { \eta ^ { 2 } } { \bar { \gamma } ^ { 2 } } - \frac { e ^ { \alpha \gamma } + e ^ { - \alpha \gamma } } { \alpha ^ { 2 } } \cdot \frac { \eta ^ { 2 } } { \bar { \gamma } ^ { 2 } } \right) .
$$

Inverting this Laplace transform, conditioned on $z \in [ - \bar { \gamma } , + \bar { \gamma } ] , \pi ( \cdot )$ is the trapezoid distribution with the following conditional density:

$$
\frac { \pi _ { + } } { \pi _ { 0 } } \cdot \frac { \eta } { \bar { \gamma } } \cdot \Big [ u ( z + \bar { \gamma } ) - u ( z - \bar { \gamma } ) + \frac { \eta } { \bar { \gamma } } \cdot ( r ( z + \bar { \gamma } ) + r ( z - \bar { \gamma } ) - r ( z + \gamma ) - r ( z - \gamma ) ) \Big ] ,
$$

where we use the standard notation $u ( \cdot ) , r ( \cdot )$ for the unit and ramp functions, respectively.

Moreover, we must have

$$
1 = \operatorname* { l i m } _ { \alpha \to 0 } \mathsf { E } _ { \boldsymbol { \pi } } \left[ e ^ { - \alpha z } | z \in [ - \bar { \gamma } , + \bar { \gamma } ] \right] = \frac { \pi _ { + } } { \pi _ { 0 } } \cdot \frac { \eta } { \bar { \gamma } } \cdot \left( 2 \bar { \gamma } + \frac { \eta } { \bar { \gamma } } ( \bar { \gamma } ^ { 2 } - \gamma ^ { 2 } ) \right) .
$$

Combining with the fact that $\pi _ { 0 } + 2 \pi _ { + } = 1$ , the result follows.

## G. Proof of Theorems 6 and 7

Proof of Theorem 6. Fix $P > 0 .$ We continue from the proof in Appendix E. Note that the re-defined ARB formula has $g _ { + } , g _ { - }$ which are not dependent on $z ;$ therefore, when differentiating with respect to z, these terms will no longer be there. We only have the mismatch between the boundary $( \pm \bar { \gamma } )$ and the expressions inside $( \mathrm { w i t h } \pm \gamma )$ , basically, along with the modified stationary distribution. Note that, from the definitions of $A _ { + } ( P , \cdot )$ and $A _ { - } ( P , \cdot )$ , it is easy to see that

$$
\bar { A } ( P , 0 ) = 0 , \qquad \bar { A } ( P , x ) \geq 0 , \forall x \geq 0 ,\tag{46}
$$

$$
\partial _ { x } \bar { A } ( P , 0 ) = P \times \frac { y ^ { * \prime } \left( P e ^ { - \gamma - \delta } \right) + e ^ { + \gamma + \delta } \cdot y ^ { * \prime } \left( P e ^ { + \gamma + \delta } \right) } { 2 } \times ( 1 - e ^ { - \delta } ) .\tag{47}
$$

Define the Laplace transform

$$
F ( s ) = \int _ { 0 } ^ { \infty } { \bar { A } } ( P , x ) e ^ { - s x } d x ,\tag{48}
$$

for $s \in \mathbb { R }$ . Observe that, from (10),

$$
{ \overline { { \mathsf { A R B } } } } = \lambda { \mathsf { P } } _ { \mathrm { t r a d e } } { \frac { \sqrt { 2 \lambda } } { \sigma } } F \left( { \frac { \sqrt { 2 \lambda } } { \sigma } } \right) .\tag{49}
$$

Applying the derivative formula for Laplace transforms (integration-by-parts) to (48), and using (46),

$$
s F ( s ) = \underbrace { \bar { A } ( P , 0 ) } _ { = 0 } + \int _ { 0 } ^ { \infty } e ^ { - s x } \partial _ { x } \bar { A } ( P , x ) d x .
$$

Observe that $s F ( s )$ is the Laplace transform of the function $\partial _ { x } { \bar { A } } ( P , \cdot )$ . Then, applying the initial value theorem for Laplace transforms18 and (47),

$$
\operatorname* { l i m } _ { s \to \infty } s \times s F ( s ) = \operatorname* { l i m } _ { x \to 0 } \partial _ { x } { \bar { A } } ( P , x ) = P \times { \frac { y ^ { * \prime } \left( P e ^ { - \gamma - \delta } \right) + e ^ { + \gamma + \delta } \cdot y ^ { * \prime } \left( P e ^ { + \gamma + \delta } \right) } { 2 } } \times ( 1 - e ^ { - \delta } )
$$

Comparing with (49),

$$
\begin{array}{c} P \times { \frac { y ^ { * \prime } \left( P e ^ { - \gamma - \delta } \right) + e ^ { + \gamma + \delta } \cdot y ^ { * \prime } \left( P e ^ { + \gamma + \delta } \right) } { 2 } } \times ( 1 - e ^ { - \delta } ) = \operatorname* { l i m } _ { \lambda \to \infty } \left( { \frac { \sqrt { 2 \lambda } } { \sigma } } \right) ^ { 2 } F \left( { \frac { \sqrt { 2 \lambda } } { \sigma } } \right)  \\ { = \operatorname* { l i m } _ { \lambda \to \infty } { \frac { \overline { { \alpha } } \overline { { \beta } } } { \sigma ^ { 2 } / 2 \times \left( { \sqrt { 2 \lambda } } / \sigma \right) \times \mathrm { P } _ { \mathrm { t r a d e } } } } . } \end{array}
$$

The result follows.

Proof of Theorem 7. We will follow the proof of Theorem 6. Fix $P > 0$ . Note that, from the definitions of $F _ { + } ( P , \cdot )$ and $F _ { - } ( P , \cdot )$ , it is easy to see that

$$
\begin{array} { r l } & { \bar { F } ( P , 0 ) = \left( 1 - e ^ { \gamma } \right) \times \frac { P \cdot \left( x ^ { * } \left( P e ^ { \gamma + \delta } \right) - x ^ { * } \left( P e ^ { \gamma } \right) \right) + y ^ { * } \left( P e ^ { - \gamma - \delta } \right) - y ^ { * } \left( P e ^ { - \gamma } \right) } { 2 } , } \\ & { \bar { F } ( P , x ) \geq 0 , \forall x \geq 0 . } \end{array}\tag{50}
$$

Define the Laplace transform

$$
G ( s ) = \int _ { 0 } ^ { \infty } { \bar { F } } ( P , x ) e ^ { - s x } d x ,\tag{51}
$$

for $s \in \mathbb { R }$ . Observe that, from (11),

$$
\overline { { { \mathsf { F E E } } } } = \lambda \mathsf { P } _ { \mathrm { t r a d e } } \frac { \sqrt { 2 \lambda } } { \sigma } G \left( \frac { \sqrt { 2 \lambda } } { \sigma } \right) .\tag{52}
$$

Applying the initial value theorem for Laplace transforms19 and (50), we get that

$$
\operatorname* { l i m } _ { s \to \infty } s G ( s ) = \operatorname* { l i m } _ { x \to 0 } { \bar { F } } ( P , x ) = ( 1 - e ^ { \gamma } ) \times { \frac { P \cdot \left( x ^ { * } \left( P e ^ { \gamma + \delta } \right) - x ^ { * } \left( P e ^ { \gamma } \right) \right) + y ^ { * } \left( P e ^ { - \gamma - \delta } \right) - y ^ { * } \left( P e ^ { - \gamma } \right) } { 2 } } .
$$

Comparing with (52),

$$
( 1 - e ^ { \gamma } ) \times \frac { P \cdot ( x ^ { * } ( P e ^ { \gamma + \delta } ) - x ^ { * } ( P e ^ { \gamma } ) ) + y ^ { * } ( P e ^ { - \gamma - \delta } ) - y ^ { * } ( P e ^ { - \gamma } ) } { 2 } = \operatorname* { l i m } _ { \lambda  \infty } ( \frac { \sqrt { 2 \lambda } } { \sigma } ) G ( \frac { \sqrt { 2 \lambda } } { \sigma } )
$$

The result follows.

## H. Consistency of asymptotic results with no gas fee

We start from the asymptotic case in arbitrage profits. Define $\overline { { \mathsf { A R B } } } _ { \mathrm { 0 } }$ to be the formula from Equation (14). By taking the limiting ratio of $\operatorname* { l i m } _ { \delta \to 0 ^ { + } } \operatorname* { l i m } _ { \lambda \to \infty } \frac { \overline { { \mathsf { A R B } } } } { \mathsf { A R B } _ { 0 } }$ , and showing that this is 1, we observe that Equation (19) correctly yields the same asymptotics as Equation (14) without gas fees when $\delta  0 ^ { + }$ . Specifically, the following calculation confirms this:20

$$
\operatorname* { l i m } _ { \delta \to 0 ^ { + } } \operatorname* { l i m } _ { \lambda \to \infty } ( 1 - e ^ { - \delta } ) \cdot \frac { \frac { \sqrt { 2 \lambda } } { \sigma } \left( 1 + \frac { \gamma \sqrt { 2 \lambda } } { \sigma } \right) } { 1 + \frac { ( \gamma + \delta ) \sqrt { 2 \lambda } } { \sigma } + \frac { \lambda } { \sigma ^ { 2 } } \cdot ( ( \gamma + \delta ) ^ { 2 } - \gamma ^ { 2 } ) } = \operatorname* { l i m } _ { \delta \to 0 ^ { + } } ( 1 - e ^ { - \delta } ) \cdot \frac { 2 \gamma } { ( \gamma + \delta ) ^ { 2 } - \gamma ^ { 2 } } = 1 .
$$

For the case on fees, our technique needs to be a bit different, and we need to be mindful of the limits of $\delta  0 ^ { + }$ , since the second factor in Equation (21) goes to zero, while the third to infinity. First, observe that

$$
\operatorname* { l i m } _ { \delta  0 ^ { + } } \operatorname* { l i m } _ { \lambda  \infty } \delta \cdot \lambda \mathsf { P } _ { \mathrm { t r a d e } } = \frac { \sigma ^ { 2 } } { 2 \gamma } .
$$

Then, note that

$$
\operatorname* { l i m } _ { \Delta \to 0 ^ { + } } \frac { 1 } { \delta } \cdot \frac { P \cdot \left( x ^ { * } \left( P e ^ { \gamma + \delta } \right) - x ^ { * } \left( P e ^ { \gamma } \right) \right) + y ^ { * } \left( P e ^ { - \gamma - \delta } \right) - y ^ { * } \left( P e ^ { - \gamma } \right) } { 2 } = - P e ^ { - \gamma } \cdot y ^ { * \prime } \left( P e ^ { - \gamma } \right) ,
$$

by first-order expansion. Finally, combining these two limiting equations along with the remaining terms of Equation (21) yields the matching terms of the asymptotics of Equation (16).

## I. Discussion of fixed gas costs

An alternative model would be to assume that the gas fee is a fixed cost $g \ \geq \ 0$ , paid in the numéraire, every time that a trade occurs. This alternative assumption (as opposed to keeping the boundary shifts $\delta _ { + } , \delta _ { - }$ constant) closely corresponds to our setting; specifically, it is an accurate depiction with smaller block times (we assume that we are in the fast block regime for most of the core analysis of arbitrageur profit rates) and larger fees $\gamma _ { + } , \gamma _ { - }$ − such that $\delta _ { + } / \gamma _ { + } , \delta _ { - } / \gamma _ { - }$ − are small.

In the setting of Lemma 2, with a fixed gas cost, an arbitrageur will only buy from the pool if the total profit exceeds the gas cost g, i.e., if

$$
P _ { t } \left\{ x ^ { * } \left( P _ { t } e ^ { - z _ { t - } } \right) - x ^ { * } \left( P _ { t } e ^ { - \gamma _ { + } } \right) \right\} + e ^ { + \gamma _ { + } } \left\{ y ^ { * } \left( P _ { t } e ^ { - z _ { t - } } \right) - y ^ { * } \left( P _ { t } e ^ { - \gamma _ { + } } \right) \right\} \geq g .
$$

What we would then take as $\bar { \gamma } _ { + } \geq \gamma _ { + }$ would be the value of the mispricing $z _ { t ^ { - } }$ for which the arbitrageur would break even, i.e., the unique solution to

$$
P _ { t } \left\{ x ^ { * } \left( P _ { t } e ^ { - { \bar { \gamma } } _ { + } } \right) - x ^ { * } \left( P _ { t } e ^ { - \gamma _ { + } } \right) \right\} + e ^ { + \gamma _ { + } } \left\{ y ^ { * } \left( P _ { t } e ^ { - { \bar { \gamma } } _ { + } } \right) - y ^ { * } \left( P _ { t } e ^ { - \gamma _ { + } } \right) \right\} = g .
$$

Then, the mispricing process would behave as per Assumption 3: if $z _ { t ^ { - } } > \bar { \gamma } _ { + }$ , we will have that $z _ { t } ~ = ~ \gamma _ { + }$ . In full generality, $\bar { \gamma } _ { + }$ will depend on $P _ { t }$ . Our model makes the assumption that $\bar { \gamma } _ { + }$ is constant. This is substantiated in many cases, where the asset volatility is not as high as to significantly move the boundary. The symmetric case holds for negative mispricing. We show that Assumption 3 is an appropriate assumption as in the example case of a CPMM below.

Example 2 (CPMM). The dependence of $\bar { \gamma } _ { + }$ on $P _ { t }$ is indicated as follows:

$$
\begin{array} { r l } & { \frac { P _ { t } } { \sqrt { P _ { t } } e ^ { - \bar { \gamma } _ { + } } } - \frac { P _ { t } } { \sqrt { P _ { t } } e ^ { - \gamma _ { + } } } + e ^ { \gamma _ { + } } \left( \sqrt { P _ { t } e ^ { - \bar { \gamma } _ { + } } } - \sqrt { P _ { t } e ^ { - \gamma _ { + } } } \right) = g / \sqrt { L } \Leftrightarrow } \\ & { e ^ { \bar { \gamma } _ { + } / 2 } ( 1 + e ^ { \gamma _ { + } - \bar { \gamma } _ { + } } ) - 2 e ^ { \gamma _ { + } / 2 } = \frac { g } { \sqrt { P _ { t } L } } . } \end{array}\tag{53}
$$

Plotting this dependence (via the inverse function), for example with a normalized instantaneous price of 1, a fee of 30 bps, and gas-equivalent quantity $g / \sqrt { P _ { t } L } = 2 \cdot 1 0 ^ { - 7 }$ (examples taken from calculations according to the Uniswap v2 ETH-USDC pool), we notice from Figure 8 that the boundary moves only slightly by 0.3 bps with the price within a $6 \%$ variation, and is roughly constant around 39 bps, hence $\delta _ { + } \approx 9$ bps.

![](images/20cde9a31a5d0fb9ab91d78277433b9ee45e4dfde46162a0cda43074c46dfb75.jpg)  
Figure 8: Sample plot of variation of $\bar { \gamma } _ { + }$ (in bps) with price $P _ { t }$ based on the parameter settings above.

Getting the Taylor expansion of Equation (53), we notice that for the CPMM, as $\bar { \gamma } _ { + } , \gamma _ { + } \to 0 , { ^ { 2 1 } }$

$$
\bar { \gamma } _ { + } \approx \gamma _ { + } + 2 \sqrt { \frac { g } { \sqrt { P _ { t } L } } } .
$$